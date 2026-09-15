// Scheduled encrypted backups of the control plane: a consistent SQLite
// snapshot plus the upload tree, tarred, encrypted (AES-256-GCM, chunked)
// and published to a local directory or an S3-compatible bucket. The mail
// stores live in the engine process and carry their own lifecycle; this
// covers the data this process owns. Archives are never written in
// plaintext — without MAILEZ_BACKUP_KEY the scheduler refuses to run.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"

	"mailez/backend/internal/cluster"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

const (
	flagBackupLast = "backup:last"
	archivePrefix  = "mailez-backup-"
	archiveSuffix  = ".enc"
	magic          = "MZBK1"
	chunkSize      = 1 << 20 // 1 MiB of plaintext per GCM record
)

// Engine runs the backup scheduler and archives on demand.
type Engine struct {
	DB  *gorm.DB
	Cfg core.Config
}

// New wires the engine.
func New(db *gorm.DB, cfg core.Config) *Engine {
	return &Engine{DB: db, Cfg: cfg}
}

// Configured reports whether a backup target is set.
func (e *Engine) Configured() bool {
	t := strings.TrimSpace(e.Cfg.BackupTarget)
	return t != "" && t != "off"
}

// Run ticks until cancelled; the archive happens once per day inside the
// configured hour.
func (e *Engine) Run(ctx context.Context) {
	if !e.Configured() {
		return
	}
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.flush()
		}
	}
}

func (e *Engine) flush() {
	// Singleton across replicas.
	if !cluster.TryHold(e.DB, "backup_run", cluster.LeaseTTL) {
		return
	}
	now := time.Now()
	if now.Hour() != e.Cfg.BackupHour {
		return
	}
	var f models.SystemFlag
	if err := e.DB.Where(&models.SystemFlag{Key: flagBackupLast}).First(&f).Error; err == nil && f.Value == now.Format("2006-01-02") {
		return
	}
	if err := e.RunNow(context.Background()); err != nil {
		// The failed run is recorded in the history; retry on the next
		// tick while the hour window lasts.
		log.Printf("backup: %v", err)
		return
	}
	e.DB.Save(&models.SystemFlag{Key: flagBackupLast, Value: now.Format("2006-01-02")})
}

// RunNow builds, encrypts and publishes one archive, then prunes old ones.
func (e *Engine) RunNow(ctx context.Context) error {
	if !e.Configured() {
		return errors.New("backup target not configured")
	}
	// Serialize against the scheduler, manual triggers and other replicas:
	// an unfinished recent run blocks a new one.
	var unfinished int64
	if err := e.DB.Model(&models.BackupRun{}).
		Where("finished_at IS NULL AND started_at > ?", time.Now().Add(-6*time.Hour)).
		Count(&unfinished).Error; err != nil {
		return err
	}
	if unfinished > 0 {
		return errors.New("a backup is already running")
	}

	rec := models.BackupRun{StartedAt: time.Now(), Target: e.Cfg.BackupTarget}
	e.DB.Create(&rec)

	if strings.TrimSpace(e.Cfg.BackupKey) == "" {
		err := errors.New("MAILEZ_BACKUP_KEY is empty; refusing to write a plaintext archive")
		e.DB.Model(&rec).Updates(map[string]any{"finished_at": time.Now(), "ok": false, "detail": err.Error()})
		return err
	}

	name, size, err := e.archive(ctx, rec.ID)
	finished := time.Now()
	if err != nil {
		e.DB.Model(&rec).Updates(map[string]any{"finished_at": finished, "ok": false, "detail": err.Error()})
		return err
	}
	e.DB.Model(&rec).Updates(map[string]any{"finished_at": finished, "ok": true, "size": size, "detail": name})
	e.prune(ctx)
	return nil
}

// archive builds the encrypted archive and publishes it under a
// timestamped name; it returns the name and published size.
func (e *Engine) archive(ctx context.Context, runID uint) (string, int64, error) {
	tmp, err := os.MkdirTemp("", "mailez-backup")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(tmp)

	staging := filepath.Join(tmp, "archive.staging")
	size, err := e.writeArchive(ctx, staging, tmp)
	if err != nil {
		return "", 0, err
	}

	name := archivePrefix + time.Now().UTC().Format("20060102-150405") + archiveSuffix
	if err := e.publish(ctx, staging, name); err != nil {
		return "", 0, err
	}
	return name, size, nil
}

// writeArchive encrypts the tar.gz of the database snapshot and the upload
// tree into dst, returning the archive size.
func (e *Engine) writeArchive(ctx context.Context, dst string, tmpDir string) (int64, error) {
	dbPath, err := e.snapshotDB(ctx, tmpDir)
	if err != nil {
		return 0, err
	}

	src, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer src.Close()

	enc := newEncryptor(src, e.Cfg.BackupKey)
	gz, err := gzip.NewWriterLevel(enc, gzip.BestSpeed)
	if err != nil {
		return 0, err
	}
	tw := tar.NewWriter(gz)

	if err := addFile(tw, dbPath, "control.db"); err != nil {
		return 0, err
	}
	if e.Cfg.UploadDir != "" {
		if err := addTree(tw, e.Cfg.UploadDir, "files"); err != nil {
			return 0, err
		}
	}

	for _, w := range []interface{ Close() error }{tw, gz, enc} {
		if err := w.Close(); err != nil {
			return 0, err
		}
	}
	info, err := src.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// snapshotDB produces a consistent copy of the control-plane database. The
// sqlite path uses VACUUM INTO, which snapshots while the live connection
// keeps serving; other drivers must be dumped natively by the operator.
func (e *Engine) snapshotDB(ctx context.Context, dir string) (string, error) {
	if e.Cfg.DBDriver != "sqlite" {
		return "", fmt.Errorf("automatic snapshots support the sqlite control plane; dump %s natively and mount it via MAILEZ_BACKUP_TARGET=local", e.Cfg.DBDriver)
	}
	out := filepath.Join(dir, "control.db")
	dsn := e.Cfg.DBDSN
	if i := strings.Index(dsn, "?"); i >= 0 {
		dsn = dsn[:i] // strip pragmas; VACUUM INTO needs a plain path
	}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return "", fmt.Errorf("open database for snapshot: %w", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	if err := db.WithContext(ctx).Exec("VACUUM INTO ?", out).Error; err != nil {
		return "", fmt.Errorf("snapshot database: %w", err)
	}
	return out, nil
}

// addFile appends one regular file under the given archive name.
func addFile(tw *tar.Writer, path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o600, Size: info.Size(), ModTime: info.ModTime(),
	}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

// addTree appends a directory tree under prefix, skipping symlinks.
func addTree(tw *tar.Writer, root, prefix string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return addFile(tw, path, prefix+"/"+filepath.ToSlash(rel))
	})
}

// encryptor is a chunked AES-256-GCM stream: magic, then records of
// [4-byte big-endian plaintext length][12-byte nonce][ciphertext+tag]. The
// key is SHA-256 of the configured passphrase — deployments should set a
// long random secret, which makes brute-forcing the derivation moot.
type encryptor struct {
	w       io.Writer
	key     []byte
	buf     [chunkSize]byte
	pending int
	err     error
}

func newEncryptor(w io.Writer, passphrase string) *encryptor {
	key := sha256.Sum256([]byte(passphrase))
	e := &encryptor{w: w, key: key[:]}
	// The header write is unconditional; surface its error on the next
	// Write so callers only need the final Close.
	if _, err := e.w.Write([]byte(magic)); err != nil {
		e.err = err
	}
	return e
}

func (e *encryptor) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	written := 0
	for len(p) > 0 {
		n := copy(e.buf[e.pending:], p)
		e.pending += n
		written += n
		p = p[n:]
		if e.pending == len(e.buf) {
			if err := e.flushChunk(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (e *encryptor) flushChunk() error {
	if e.pending == 0 {
		return nil
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(e.pending))
	ct := gcm.Seal(nil, nonce, e.buf[:e.pending], nil)
	for _, part := range [][]byte{header[:], nonce, ct} {
		if _, err := e.w.Write(part); err != nil {
			e.err = err
			return err
		}
	}
	e.pending = 0
	return nil
}

func (e *encryptor) Close() error {
	if e.err != nil {
		return e.err
	}
	return e.flushChunk()
}

// decryptor replays the encryptor's format.
type decryptor struct {
	r       io.Reader
	gcm     cipher.AEAD
	buf     []byte
	pending int
}

func newDecryptor(r io.Reader, passphrase string) (*decryptor, error) {
	key := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	magicBuf := make([]byte, len(magic))
	if _, err := io.ReadFull(r, magicBuf); err != nil {
		return nil, err
	}
	if string(magicBuf) != magic {
		return nil, errors.New("not a mailez backup archive")
	}
	return &decryptor{r: r, gcm: gcm}, nil
}

func (d *decryptor) next() ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(d.r, header[:]); err != nil {
		return nil, io.EOF
	}
	n := binary.BigEndian.Uint32(header[:])
	if n > chunkSize {
		return nil, fmt.Errorf("corrupt record length %d", n)
	}
	nonce := make([]byte, d.gcm.NonceSize())
	if _, err := io.ReadFull(d.r, nonce); err != nil {
		return nil, err
	}
	ct := make([]byte, int(n)+d.gcm.Overhead())
	if _, err := io.ReadFull(d.r, ct); err != nil {
		return nil, err
	}
	pt, err := d.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong key or corrupted archive)")
	}
	return pt, nil
}

func (d *decryptor) Read(p []byte) (int, error) {
	for d.pending == 0 {
		pt, err := d.next()
		if err != nil {
			return 0, err
		}
		d.buf = pt
		d.pending = len(pt)
	}
	n := copy(p, d.buf[:d.pending])
	d.buf = d.buf[n:]
	d.pending -= n
	return n, nil
}

// publish copies the staged archive to the configured target under name.
func (e *Engine) publish(ctx context.Context, staging, name string) error {
	t := e.Cfg.BackupTarget
	switch {
	case strings.HasPrefix(t, "local:"):
		dir := strings.TrimPrefix(t, "local:")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		return copyFile(staging, filepath.Join(dir, name))
	case t == "s3":
		return e.publishS3(ctx, staging, name)
	default:
		return fmt.Errorf("unknown backup target %q (use local:<dir> or s3)", t)
	}
}

func (e *Engine) publishS3(ctx context.Context, staging, name string) error {
	if e.Cfg.MinioEndpoint == "" {
		return errors.New("s3 target selected but MAILEZINE_S3_ENDPOINT is unset")
	}
	f, err := os.Open(staging)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	client, err := minio.New(e.Cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(e.Cfg.MinioAccessKey, e.Cfg.MinioSecretKey, ""),
		Secure: e.Cfg.MinioUseSSL,
	})
	if err != nil {
		return err
	}
	_, err = client.PutObject(ctx, e.Cfg.MinioBucket, "mailez-backup/"+name, f, info.Size(),
		minio.PutObjectOptions{ContentType: "application/octet-stream"})
	return err
}

// listArchives returns the archive names currently on the target, oldest
// first.
func (e *Engine) listArchives(ctx context.Context) ([]string, error) {
	t := e.Cfg.BackupTarget
	var names []string
	switch {
	case strings.HasPrefix(t, "local:"):
		entries, err := os.ReadDir(strings.TrimPrefix(t, "local:"))
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		for _, ent := range entries {
			if !ent.IsDir() && strings.HasPrefix(ent.Name(), archivePrefix) && strings.HasSuffix(ent.Name(), archiveSuffix) {
				names = append(names, ent.Name())
			}
		}
	case t == "s3":
		client, err := e.s3(ctx)
		if err != nil {
			return nil, err
		}
		objects := client.ListObjects(ctx, e.Cfg.MinioBucket, minio.ListObjectsOptions{Prefix: "mailez-backup/", Recursive: true})
		for obj := range objects {
			if obj.Err != nil {
				return nil, obj.Err
			}
			names = append(names, strings.TrimPrefix(obj.Key, "mailez-backup/"))
		}
	}
	sort.Strings(names)
	return names, nil
}

// prune deletes archives beyond the configured retention.
func (e *Engine) prune(ctx context.Context) {
	keep := e.Cfg.BackupKeep
	if keep <= 0 {
		keep = 14
	}
	names, err := e.listArchives(ctx)
	if err != nil {
		log.Printf("backup: list for prune: %v", err)
		return
	}
	for _, name := range names {
		if len(names) <= keep {
			break
		}
		if err := e.remove(ctx, name); err != nil {
			log.Printf("backup: prune %s: %v", name, err)
			continue
		}
		names = names[1:]
	}
}

func (e *Engine) remove(ctx context.Context, name string) error {
	t := e.Cfg.BackupTarget
	if strings.HasPrefix(t, "local:") {
		return os.Remove(filepath.Join(strings.TrimPrefix(t, "local:"), name))
	}
	if t == "s3" {
		client, err := e.s3(ctx)
		if err != nil {
			return err
		}
		return client.RemoveObject(ctx, e.Cfg.MinioBucket, "mailez-backup/"+name, minio.RemoveObjectOptions{})
	}
	return fmt.Errorf("unknown backup target %q", t)
}

func (e *Engine) s3(ctx context.Context) (*minio.Client, error) {
	if e.Cfg.MinioEndpoint == "" {
		return nil, errors.New("s3 target selected but MAILEZINE_S3_ENDPOINT is unset")
	}
	return minio.New(e.Cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(e.Cfg.MinioAccessKey, e.Cfg.MinioSecretKey, ""),
		Secure: e.Cfg.MinioUseSSL,
	})
}

// Verify re-reads an archived run from the target: decrypts the stream and
// walks the tar, proving the key, the transport and the container all
// survived. It returns the number of entries seen.
func (e *Engine) Verify(ctx context.Context, runID uint) (int, error) {
	var rec models.BackupRun
	if err := e.DB.First(&rec, runID).Error; err != nil {
		return 0, err
	}
	name := rec.Detail
	if !strings.HasPrefix(name, archivePrefix) {
		return 0, fmt.Errorf("run %d has no archive name recorded", runID)
	}

	t := e.Cfg.BackupTarget
	var src io.ReadCloser
	switch {
	case strings.HasPrefix(t, "local:"):
		f, err := os.Open(filepath.Join(strings.TrimPrefix(t, "local:"), name))
		if err != nil {
			return 0, err
		}
		src = f
	case t == "s3":
		client, err := e.s3(ctx)
		if err != nil {
			return 0, err
		}
		obj, err := client.GetObject(ctx, e.Cfg.MinioBucket, "mailez-backup/"+name, minio.GetObjectOptions{})
		if err != nil {
			return 0, err
		}
		src = obj
	default:
		return 0, fmt.Errorf("unknown backup target %q", t)
	}
	defer src.Close()

	dec, err := newDecryptor(src, e.Cfg.BackupKey)
	if err != nil {
		return 0, err
	}
	gz, err := gzip.NewReader(dec)
	if err != nil {
		return 0, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := 0
	for {
		if _, err := tr.Next(); err != nil {
			if err == io.EOF {
				return entries, nil
			}
			return entries, err
		}
		entries++
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return entries, err
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

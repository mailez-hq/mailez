package drive

// Blob storage backends for the cloud drive: local disk (default) or
// an S3-compatible object store (RustFS, MinIO, cloud S3). Files are
// addressed by an opaque key (StoredPath).
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"mailez/backend/internal/core"
)

// Store persists and retrieves drive blobs by key.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// s3Configured reports whether DriveBackend selects the S3-compatible blob
// store. "minio" stays accepted as the legacy spelling of "s3".
func s3Configured(backend string) bool {
	return strings.EqualFold(backend, "s3") ||
		strings.EqualFold(backend, "minio") ||
		strings.EqualFold(backend, "rustfs")
}

// NewStore builds the configured backend.
func NewStore(cfg core.Config) (Store, error) {
	if s3Configured(cfg.DriveBackend) {
		return newS3Store(cfg)
	}
	root := cfg.UploadDir
	if root == "" {
		root = "uploads"
	}
	return &LocalStore{Root: filepath.Join(root, "drive")}, nil
}

// NewUploadsStore builds the blob backend for the large-attachment relay:
// the same S3-compatible backend as the drive when configured, but the
// local root stays the upload dir so keys remain the on-disk relative
// paths existing deployments already have.
func NewUploadsStore(cfg core.Config) (Store, error) {
	if s3Configured(cfg.DriveBackend) {
		return newS3Store(cfg)
	}
	root := cfg.UploadDir
	if root == "" {
		root = "uploads"
	}
	return &LocalStore{Root: root}, nil
}

// LocalStore keeps blobs under the configured data directory.
type LocalStore struct {
	Root string
}

func (s *LocalStore) path(key string) string {
	return filepath.Join(s.Root, filepath.FromSlash(key))
}

func (s *LocalStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	dst := s.path(key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

func (s *LocalStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(s.path(key))
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	err := os.Remove(s.path(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// S3Store stores blobs in an S3-compatible bucket (RustFS, MinIO, cloud S3).
type S3Store struct {
	client *minio.Client
	bucket string
}

func newS3Store(cfg core.Config) (Store, error) {
	client, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}
	bucket := cfg.S3Bucket
	if bucket == "" {
		bucket = "mailezine"
	}
	if err := client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{}); err != nil {
		// AlreadyExists is fine; other errors surface on first Put.
		exists, _ := client.BucketExists(context.Background(), bucket)
		if !exists {
			return nil, fmt.Errorf("s3 bucket %s: %w", bucket, err)
		}
	}
	return &S3Store{client: client, bucket: bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// newBlobKey derives a storage key from the file content hash plus a random
// suffix, keeping blobs immutable and content-addressed.
func newBlobKey(userEmail, filename string, digest []byte) string {
	return "drive/" + userEmail + "/" + hex.EncodeToString(digest)[:16] + "-" + safeName(filename)
}

func safeName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		return "file.bin"
	}
	return name
}

func sha256Reader(r io.Reader) ([]byte, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

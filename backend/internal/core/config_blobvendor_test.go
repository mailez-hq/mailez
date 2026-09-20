package core

import "testing"

func TestBlobVendor(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"local fs", Config{BlobBackend: "local", S3Endpoint: "rustfs:9000"}, ""},
		{"compose rustfs service", Config{BlobBackend: "s3", S3Endpoint: "rustfs:9000"}, "rustfs"},
		{"compose minio service", Config{BlobBackend: "s3", S3Endpoint: "minio:9000"}, "minio"},
		{"explicit drive backend wins", Config{BlobBackend: "s3", DriveBackend: "rustfs", S3Endpoint: "s3.internal:9000"}, "rustfs"},
		{"host with port", Config{BlobBackend: "s3", S3Endpoint: "rustfs-1.example.com:9000"}, "rustfs"},
		{"scheme prefixed", Config{BlobBackend: "s3", S3Endpoint: "https://MinIO.example.com/"}, "minio"},
		{"cloud endpoint stays neutral", Config{BlobBackend: "s3", S3Endpoint: "s3.us-east-1.amazonaws.com"}, ""},
	}
	for _, tc := range cases {
		if got := tc.cfg.BlobVendor(); got != tc.want {
			t.Errorf("%s: BlobVendor() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

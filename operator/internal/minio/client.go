/*
Copyright 2026 Velora Authors.
Licensed under the Apache License, Version 2.0.
*/

package minio

import (
	"context"
	"fmt"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client is the interface the reconciler uses to interact with MinIO.
// Defined as an interface so tests can substitute a fake.
type Client interface {
	// EnsureBucket creates the bucket if it does not exist.
	// Returns (true, nil) if created, (false, nil) if already exists, (false, err) on failure.
	EnsureBucket(ctx context.Context, bucketName string) (bool, error)

	// BucketExists returns true if the named bucket exists.
	BucketExists(ctx context.Context, bucketName string) (bool, error)
}

// minioClient wraps the minio-go SDK and implements Client.
type minioClient struct {
	mc       *miniogo.Client
	location string // S3 region / MinIO location (default: "us-east-1")
}

// Config holds connection settings for the MinIO client.
type Config struct {
	Endpoint        string // e.g. "minio.minio.svc.cluster.local:9000"
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	Location        string // bucket region; defaults to "us-east-1"
}

// NewClient creates and returns a new MinIO Client.
func NewClient(cfg Config) (Client, error) {
	mc, err := miniogo.New(cfg.Endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	location := cfg.Location
	if location == "" {
		location = "us-east-1"
	}

	return &minioClient{mc: mc, location: location}, nil
}

// EnsureBucket creates the bucket if it does not already exist.
// This is the key operation called by the reconciler on every reconcile loop —
// it must be idempotent.
func (c *minioClient) EnsureBucket(ctx context.Context, bucketName string) (bool, error) {
	exists, err := c.mc.BucketExists(ctx, bucketName)
	if err != nil {
		return false, fmt.Errorf("checking bucket existence: %w", err)
	}
	if exists {
		// Idempotent no-op — bucket already present.
		return false, nil
	}

	if err := c.mc.MakeBucket(ctx, bucketName, miniogo.MakeBucketOptions{
		Region: c.location,
	}); err != nil {
		return false, fmt.Errorf("creating bucket %q: %w", bucketName, err)
	}

	return true, nil
}

// BucketExists returns true if the named bucket exists.
func (c *minioClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	return c.mc.BucketExists(ctx, bucketName)
}

// ---------------------------------------------------------------------------
// Fake client for testing
// ---------------------------------------------------------------------------

// FakeClient is an in-memory MinIO client for unit tests.
type FakeClient struct {
	Buckets     map[string]bool
	EnsureError error // if set, EnsureBucket returns this error
}

// NewFakeClient creates a FakeClient with an empty bucket store.
func NewFakeClient() *FakeClient {
	return &FakeClient{Buckets: make(map[string]bool)}
}

func (f *FakeClient) EnsureBucket(_ context.Context, bucketName string) (bool, error) {
	if f.EnsureError != nil {
		return false, f.EnsureError
	}
	if f.Buckets[bucketName] {
		return false, nil // already exists
	}
	f.Buckets[bucketName] = true
	return true, nil // created
}

func (f *FakeClient) BucketExists(_ context.Context, bucketName string) (bool, error) {
	return f.Buckets[bucketName], nil
}

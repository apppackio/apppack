package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

type fakeObjectDownloader struct {
	input   *transfermanager.DownloadObjectInput
	content string
	err     error
}

func (f *fakeObjectDownloader) DownloadObject(
	_ context.Context,
	input *transfermanager.DownloadObjectInput,
	_ ...func(*transfermanager.Options),
) (*transfermanager.DownloadObjectOutput, error) {
	f.input = input

	if f.err != nil {
		return nil, f.err
	}

	if _, err := input.WriterAt.WriteAt([]byte(f.content), 0); err != nil {
		return nil, err
	}

	return &transfermanager.DownloadObjectOutput{}, nil
}

type fakeObjectUploader struct {
	input *transfermanager.UploadObjectInput
	err   error
}

func (f *fakeObjectUploader) UploadObject(
	_ context.Context,
	input *transfermanager.UploadObjectInput,
	_ ...func(*transfermanager.Options),
) (*transfermanager.UploadObjectOutput, error) {
	f.input = input

	if f.err != nil {
		return nil, f.err
	}

	return &transfermanager.UploadObjectOutput{}, nil
}

// TestDownloadObjectWritesToFile checks that the bucket and key of the dump we
// asked for reach the transfer manager, and that what it hands back lands in
// the file the user named.
func TestDownloadObjectWritesToFile(t *testing.T) {
	t.Parallel()

	fake := &fakeObjectDownloader{content: "dump contents"}
	outputFile := filepath.Join(t.TempDir(), "app.dump")

	err := downloadObject(fake, &s3.GetObjectInput{
		Bucket: aws.String("dbutils-bucket"),
		Key:    aws.String("dumps/20260101000000-pete.dump"),
	}, outputFile)
	require.NoError(t, err)

	require.Equal(t, "dbutils-bucket", *fake.input.Bucket)
	require.Equal(t, "dumps/20260101000000-pete.dump", *fake.input.Key)

	contents, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	require.Equal(t, "dump contents", string(contents))
}

func TestDownloadObjectReturnsError(t *testing.T) {
	t.Parallel()

	fake := &fakeObjectDownloader{err: errors.New("access denied")}

	err := downloadObject(fake, &s3.GetObjectInput{
		Bucket: aws.String("dbutils-bucket"),
		Key:    aws.String("dumps/20260101000000-pete.dump"),
	}, filepath.Join(t.TempDir(), "app.dump"))
	require.ErrorContains(t, err, "access denied")
}

func TestUploadObject(t *testing.T) {
	t.Parallel()

	fake := &fakeObjectUploader{}
	input := &transfermanager.UploadObjectInput{
		Bucket: aws.String("dbutils-bucket"),
		Key:    aws.String("uploads/20260101000000-pete.dump"),
		Body:   strings.NewReader("dump contents"),
	}

	require.NoError(t, uploadObject(fake, input))
	require.Same(t, input, fake.input)
}

func TestUploadObjectReturnsError(t *testing.T) {
	t.Parallel()

	fake := &fakeObjectUploader{err: errors.New("access denied")}

	err := uploadObject(fake, &transfermanager.UploadObjectInput{
		Bucket: aws.String("dbutils-bucket"),
		Key:    aws.String("uploads/20260101000000-pete.dump"),
		Body:   strings.NewReader("dump contents"),
	})
	require.ErrorContains(t, err, "access denied")
}

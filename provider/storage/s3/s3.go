package s3

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4" //nolint:revive // goimports insists on aliasing versioned import paths
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/pkg/errors"

	storepb "github.com/usememos/memos/proto/gen/store"
)

// ObjectStream contains the original bytes and size of an object.
type ObjectStream struct {
	Body io.ReadCloser
	// ContentLength is the number of bytes in Body, or -1 when unknown.
	ContentLength int64
}

// Driver reads legacy attachments from an S3-compatible object store.
type Driver struct {
	Client *s3.Client
	bucket *string
}

// NewDriver creates a read-only storage driver from a legacy S3 configuration.
func NewDriver(ctx context.Context, s3Config *storepb.StorageS3Config) (*Driver, error) {
	loadOptions := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s3Config.AccessKeyId, s3Config.AccessKeySecret, "")),
		config.WithRegion(s3Config.Region),
	}
	if s3Config.InsecureSkipTlsVerify {
		// Skip TLS certificate verification for endpoints using self-signed certificates.
		// This is opt-in and removes protection against man-in-the-middle attacks.
		httpClient := awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- opt-in for self-signed S3 endpoints
		})
		loadOptions = append(loadOptions, config.WithHTTPClient(httpClient))
	}

	cfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load s3 config")
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s3Config.Endpoint)
		o.UsePathStyle = s3Config.UsePathStyle
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		o.APIOptions = append(o.APIOptions, excludeAcceptEncodingFromSigning, forceSignedPayload)
	})
	return &Driver{
		Client: client,
		bucket: aws.String(s3Config.Bucket),
	}, nil
}

type acceptEncodingKey struct{}

type dropAcceptEncoding struct{}

func (dropAcceptEncoding) ID() string { return "MemosDropAcceptEncoding" }

func (dropAcceptEncoding) HandleFinalize(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
	if req, ok := in.Request.(*smithyhttp.Request); ok {
		if values := req.Header.Values("Accept-Encoding"); len(values) > 0 {
			ctx = context.WithValue(ctx, acceptEncodingKey{}, values)
			req.Header.Del("Accept-Encoding")
		}
	}
	return next.HandleFinalize(ctx, in)
}

type restoreAcceptEncoding struct{}

func (restoreAcceptEncoding) ID() string { return "MemosRestoreAcceptEncoding" }

func (restoreAcceptEncoding) HandleFinalize(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
	if req, ok := in.Request.(*smithyhttp.Request); ok {
		if values, ok := ctx.Value(acceptEncodingKey{}).([]string); ok {
			for _, value := range values {
				req.Header.Add("Accept-Encoding", value)
			}
		}
	}
	return next.HandleFinalize(ctx, in)
}

// excludeAcceptEncodingFromSigning keeps the Accept-Encoding header out of the
// SigV4 signature while still sending it on the wire. Some S3-compatible
// providers (notably Google Cloud Storage) rewrite the header in transit, so a
// signature covering it never verifies and requests fail with
// SignatureDoesNotMatch.
func excludeAcceptEncodingFromSigning(stack *middleware.Stack) error {
	if err := stack.Finalize.Insert(dropAcceptEncoding{}, "Signing", middleware.Before); err != nil {
		return err
	}
	return stack.Finalize.Insert(restoreAcceptEncoding{}, "Signing", middleware.After)
}

// forceSignedPayload signs request payloads with their real SHA-256 instead of
// the UNSIGNED-PAYLOAD marker the SDK uses over TLS, which some S3-compatible
// providers (notably Google Cloud Storage) reject.
func forceSignedPayload(stack *middleware.Stack) error {
	_, err := stack.Finalize.Swap((*v4.ComputePayloadSHA256)(nil).ID(), &v4.ComputePayloadSHA256{})
	return err
}

// GetObjectStream opens a complete object. The caller closes the stream body.
func (c *Driver) GetObjectStream(ctx context.Context, key string) (*ObjectStream, error) {
	output, err := c.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: c.bucket,
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get object")
	}
	stream := &ObjectStream{
		Body:          output.Body,
		ContentLength: -1,
	}
	if output.ContentLength != nil {
		stream.ContentLength = *output.ContentLength
	}
	return stream, nil
}

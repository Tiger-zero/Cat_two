package api

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	filterpkg "github.com/usememos/memos/filter"
)

func prepareMemoFilter(ctx context.Context, filterText string) (*filterpkg.Program, error) {
	engine, err := filterpkg.DefaultEngine()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to initialize filter: %v", err)
	}
	return prepareFilter(ctx, filterText, engine)
}

func prepareAttachmentFilter(ctx context.Context, filterText string) (*filterpkg.Program, error) {
	engine, err := filterpkg.DefaultAttachmentEngine()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to initialize filter: %v", err)
	}
	return prepareFilter(ctx, filterText, engine)
}

func prepareFilter(ctx context.Context, filterText string, engine *filterpkg.Engine) (*filterpkg.Program, error) {
	program, err := engine.Compile(ctx, filterText)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
	}
	if _, err := program.Render(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
	}

	return program, nil
}

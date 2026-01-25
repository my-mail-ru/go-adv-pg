package advpgconn

import "context"

type QueryInfo struct {
	Table string
	Index string
}

type ctxQueryInfo struct{}

func (qi *QueryInfo) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxQueryInfo{}, qi)
}

func QueryInfoFromContext(ctx context.Context) *QueryInfo {
	if qi := ctx.Value(ctxQueryInfo{}); qi != nil {
		return qi.(*QueryInfo)
	}

	return nil
}

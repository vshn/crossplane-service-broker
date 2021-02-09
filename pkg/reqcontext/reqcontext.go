package reqcontext

import (
	"context"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
)

// ReqContext provides a simple struct to contain required request scoped context. In doing so we avoid
// stuffing everything into `req.Context()` which contains just untyped values.
// See e.g. https://dave.cheney.net/2017/01/26/context-is-for-cancelation why this would be a bad idea.
type ReqContext struct {
	Context       context.Context
	CorrelationID string
	Logger        lager.Logger
}

// NewReqContext reads data from context and sets up the request context.
func NewReqContext(ctx context.Context, logger lager.Logger, logData lager.Data) *ReqContext {
	if logData == nil {
		logData = lager.Data{}
	}
	id, ok := ctx.Value(middlewares.CorrelationIDKey).(string)
	if !ok {
		id = "unknown"
	}
	logData["correlation-id"] = id

	return &ReqContext{
		Context:       ctx,
		CorrelationID: id,
		Logger:        logger.WithData(logData),
	}
}

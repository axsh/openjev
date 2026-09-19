package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"openjev/features/decision-test/internal/decision"
	"openjev/features/decision-test/internal/domain"
)

type systemoneInput struct {
	RawBody []byte
}

type systemoneOutput struct {
	Body domain.Response
}

type healthOutput struct {
	Status int
	Body   domain.Health
}

func Register(humaAPI huma.API, svc *decision.Service, health func(context.Context) domain.Health) {
	huma.Register(humaAPI, huma.Operation{
		OperationID: "post-systemone",
		Method:      http.MethodPost,
		Path:        "/v1/systemone",
	}, func(ctx context.Context, input *systemoneInput) (*systemoneOutput, error) {
		if svc == nil {
			return nil, huma.Error503ServiceUnavailable("decision service is not ready")
		}
		start := time.Now()
		req, err := domain.Validate(input.RawBody, svc.ModelID)
		if err != nil {
			svc.Log.Debug("request rejected", "error", err)
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		svc.Log.Debug("systemone accepted", "questions", len(req.Questions), "method", req.Method)
		resp, err := svc.Run(ctx, req)
		if err != nil {
			if errors.Is(err, decision.ErrEngine) {
				return nil, huma.Error502BadGateway(err.Error())
			}
			return nil, err
		}
		resp.ElapsedMs = time.Since(start).Milliseconds()
		return &systemoneOutput{Body: resp}, nil
	})

	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/health",
	}, func(ctx context.Context, input *struct{}) (*healthOutput, error) {
		body := health(ctx)
		status := http.StatusOK
		if !body.Ready {
			status = http.StatusServiceUnavailable
		}
		return &healthOutput{Status: status, Body: body}, nil
	})
}

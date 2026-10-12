// Package httpapi exposes the engine through a Hertz HTTP/SSE API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"

	"github.com/izhw/ai-agent/agentkit"
	"github.com/izhw/ai-agent/contract"
)

const (
	headerTenant  = "X-Tenant-ID"
	headerProduct = "X-Product-ID"
	headerUser    = "X-User-ID"
)

type Server struct{ engine *agentkit.Engine }

func New(engine *agentkit.Engine) *Server { return &Server{engine: engine} }

func (s *Server) Register(h *server.Hertz) {
	h.GET(
		"/healthz",
		func(_ context.Context, c *app.RequestContext) { c.JSON(consts.StatusOK, utils.H{"status": "ok"}) },
	)
	h.POST("/v1/admin/agents", s.publishAgent)
	h.POST("/v1/agents/:agent_key/runs", s.startRun)
	h.GET("/v1/runs/:run_id", s.handleGetRun)
	h.GET("/v1/runs/:run_id/events", s.listEvents)
	h.POST("/v1/runs/:run_id/resume", s.resumeRun)
}

func (s *Server) publishAgent(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Spec     contract.AgentSpec `json:"spec"`
		Activate bool               `json:"activate"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, err)
		return
	}
	if body.Spec.TenantID == "" {
		body.Spec.TenantID = header(c, headerTenant)
	}
	if body.Spec.ProductID == "" {
		body.Spec.ProductID = header(c, headerProduct)
	}
	spec, err := s.engine.PublishAgent(ctx, body.Spec, body.Activate)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err)
		return
	}
	c.JSON(consts.StatusCreated, spec)
}

func (s *Server) startRun(ctx context.Context, c *app.RequestContext) {
	var body struct {
		ConversationID string            `json:"conversation_id"`
		Message        string            `json:"message"`
		Variables      map[string]any    `json:"variables"`
		Metadata       map[string]string `json:"metadata"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, err)
		return
	}
	request := contract.RunRequest{
		TenantID:       header(c, headerTenant),
		ProductID:      header(c, headerProduct),
		UserID:         header(c, headerUser),
		AgentKey:       c.Param("agent_key"),
		ConversationID: body.ConversationID,
		Message:        contract.Message{Role: contract.RoleUser, Content: body.Message},
		Variables:      body.Variables,
		Metadata:       body.Metadata,
	}
	run, stream, err := s.engine.Start(ctx, request)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err)
		return
	}
	defer stream.Close()
	if acceptsSSE(c) {
		writeSSE(ctx, c, run, stream)
		return
	}
	events, err := drain(ctx, stream)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err)
		return
	}
	c.JSON(consts.StatusOK, utils.H{"run": run, "events": events})
}

func (s *Server) resumeRun(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Targets map[string]any `json:"targets"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, err)
		return
	}
	request := contract.ResumeRequest{
		TenantID:  header(c, headerTenant),
		ProductID: header(c, headerProduct),
		UserID:    header(c, headerUser),
		RunID:     c.Param("run_id"),
		Targets:   body.Targets,
	}
	run, stream, err := s.engine.Resume(ctx, request)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err)
		return
	}
	defer stream.Close()
	if acceptsSSE(c) {
		writeSSE(ctx, c, run, stream)
		return
	}
	events, err := drain(ctx, stream)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err)
		return
	}
	c.JSON(consts.StatusOK, utils.H{"run": run, "events": events})
}

func (s *Server) handleGetRun(ctx context.Context, c *app.RequestContext) {
	run, err := s.engine.GetRun(
		ctx,
		header(c, headerTenant),
		header(c, headerProduct),
		c.Param("run_id"),
	)
	if err != nil {
		writeError(c, consts.StatusNotFound, err)
		return
	}
	c.JSON(consts.StatusOK, run)
}

func (s *Server) listEvents(ctx context.Context, c *app.RequestContext) {
	after, err := strconv.ParseInt(c.Query("after"), 10, 64)
	if err != nil {
		after = 0
	}
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 0
	}
	events, err := s.engine.Events(
		ctx,
		header(c, headerTenant),
		header(c, headerProduct),
		c.Param("run_id"),
		after,
		limit,
	)
	if err != nil {
		writeError(c, consts.StatusNotFound, err)
		return
	}
	c.JSON(consts.StatusOK, utils.H{"events": events})
}

func writeSSE(ctx context.Context, c *app.RequestContext, run contract.Run, stream interface {
	Next(context.Context) (contract.RunEvent, error)
},
) {
	c.Response.Header.Set("X-Run-ID", run.ID)
	writer := sse.NewWriter(c)
	for {
		event, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			payload, marshalErr := json.Marshal(utils.H{"error": err.Error()})
			if marshalErr != nil {
				return
			}
			if writeErr := writer.WriteEvent("", "run.failed", payload); writeErr != nil {
				return
			}
			return
		}
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return
		}
		if err = writer.WriteEvent(
			strconv.FormatInt(event.Sequence, 10),
			string(event.Type),
			payload,
		); err != nil {
			return
		}
	}
}

func drain(ctx context.Context, stream interface {
	Next(context.Context) (contract.RunEvent, error)
},
) ([]contract.RunEvent, error) {
	var result []contract.RunEvent
	for {
		event, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			return result, err
		}
		result = append(result, event)
	}
}

func acceptsSSE(c *app.RequestContext) bool {
	return strings.Contains(strings.ToLower(header(c, "Accept")), "text/event-stream")
}
func header(c *app.RequestContext, name string) string { return string(c.GetHeader(name)) }
func writeError(c *app.RequestContext, status int, err error) {
	c.JSON(status, utils.H{"error": err.Error()})
}

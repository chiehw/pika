package handler

import (
	"github.com/go-orz/orz"
	"github.com/labstack/echo/v5"
	"github.com/pika-monitor/pika/internal/protocol"
	"github.com/pika-monitor/pika/internal/service"
)

type LogMonitorHandler struct{ service *service.LogMonitorService }

func NewLogMonitorHandler(service *service.LogMonitorService) *LogMonitorHandler {
	return &LogMonitorHandler{service: service}
}
func (h *LogMonitorHandler) GetConfig(c *echo.Context) error {
	config, err := h.service.GetConfig(c.Request().Context(), c.Param("id"))
	if err != nil {
		return err
	}
	return orz.Ok(c, config)
}
func (h *LogMonitorHandler) UpdateConfig(c *echo.Context) error {
	var req protocol.LogMonitorConfig
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.Validate(); err != nil {
		return orz.NewError(400, err.Error())
	}
	if err := h.service.UpdateConfig(c.Request().Context(), c.Param("id"), req); err != nil {
		return err
	}
	return orz.Ok(c, orz.Map{})
}

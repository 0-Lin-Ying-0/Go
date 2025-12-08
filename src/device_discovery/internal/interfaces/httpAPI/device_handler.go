package httpAPI

import (
	"device_discovery/internal/application"
	"net/http"

	"github.com/gin-gonic/gin"
)

// DeviceHandler 把 HTTP 请求转成对应用服务的调用
type DeviceHandler struct {
	svc *application.DiscoveryService
}

func NewDeviceHandler(svc *application.DiscoveryService) *DeviceHandler {
	return &DeviceHandler{
		svc: svc,
	}
}

// RegisterRoutes 注册路由
func (h *DeviceHandler) RegisterRoutes(r *gin.Engine) {
	devices := r.Group("/devices")
	devices.GET("", h.ListAllDevices)
	devices.GET("/:ip", h.GetDeviceByIP)
}

// GET /devices

func (h *DeviceHandler) ListAllDevices(c *gin.Context) {
	list, err := h.svc.ListAllDevices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /devices/:ip

func (h *DeviceHandler) GetDeviceByIP(c *gin.Context) {
	ip := c.Param("ip")

	dto, err := h.svc.GetDeviceByIP(ip)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, dto)
}

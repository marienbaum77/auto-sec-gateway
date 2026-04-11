package api

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/igor/auto-sec-manager/internal/model"
	"gorm.io/gorm"
)

type Server struct {
	db            *gorm.DB
	publicIP      string
	xrayPublicKey string
	shortID       string
	xrayPort      int
	nodeAlive     *atomic.Bool
}

func NewServer(db *gorm.DB, publicIP, xrayPublicKey, shortID string, xrayPort int, nodeAlive *atomic.Bool) *Server {
	return &Server{
		db:            db,
		publicIP:      publicIP,
		xrayPublicKey: xrayPublicKey,
		shortID:       shortID,
		xrayPort:      xrayPort,
		nodeAlive:     nodeAlive,
	}
}

func (s *Server) Register(r *gin.Engine) {
	r.GET("/sub/:token", s.handleSub)
}

func (s *Server) handleSub(c *gin.Context) {
	token := c.Param("token")
	var user model.User

	if err := s.db.Where("token = ? AND active = ?", token, true).First(&user).Error; err != nil {
		c.String(http.StatusNotFound, "Invalid or inactive subscription")
		return
	}

	if !s.nodeAlive.Load() {
		c.String(http.StatusServiceUnavailable, "Xray node is unavailable")
		return
	}

	vlessURL := fmt.Sprintf(
		"vless://%s@%s:%d?security=reality&sni=dl.google.com&fp=chrome&pbk=%s&sid=%s&type=tcp&flow=xtls-rprx-vision#Sovereign-%s",
		user.UUID, s.publicIP, s.xrayPort, s.xrayPublicKey, s.shortID, user.Username,
	)

	c.String(http.StatusOK, vlessURL)
}

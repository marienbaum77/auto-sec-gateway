package api

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/marienbaum77/auto-sec-gateway/internal/model"
	"gorm.io/gorm"
)

type Server struct {
	db       *gorm.DB
	publicIP string
}

func NewServer(db *gorm.DB, publicIP string) *Server {
	return &Server{db: db, publicIP: publicIP}
}

func (s *Server) Register(r *gin.Engine) {
	r.GET("/sub/:token", s.handleSub)
}

func (s *Server) handleSub(c *gin.Context) {
	token := c.Param("token")
	var user model.User	

	if err := s.db.Where("token = ? AND active = ?", token, true).First(&user).Error; err != nil {
		c.String(http.StatusNotFound, "Invalid subscription")
		return
	}
	
	hyPass := os.Getenv("HYSTERIA_PASSWORD")
	if hyPass == "" {
		c.String(http.StatusInternalServerError, "Ошибка: HYSTERIA_PASSWORD не задан")
		return
	}

	domain := "auto-sec-gateway.duckdns.org"

	hy2URL := fmt.Sprintf(
		"hysteria2://%s@%s:443?sni=%s&obfs=salamander&obfs-password=%s#Sovereign-%s",
		hyPass, s.publicIP, domain, hyPass, user.Username,
	)

	c.String(http.StatusOK, hy2URL)
}

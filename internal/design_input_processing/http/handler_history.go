package http

import (
	"strconv"

	handlers "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/http/handlers"
	"github.com/gin-gonic/gin"
)

func (h *Handler) chatHistory(c *gin.Context) {
	id := c.Param("id")
	nStr := c.DefaultQuery("n", "50")
	n, _ := strconv.Atoi(nStr)
	turns := handlers.ReadChat(id, n) // expose ReadChat (or make it public)
	if turns == nil {
		c.JSON(200, gin.H{"ok": true, "turns": []any{}})
		return
	}
	c.JSON(200, gin.H{"ok": true, "turns": turns})
}

func (h *Handler) chatClear(c *gin.Context) {
	id := c.Param("id")
	ok := handlers.ClearChat(id) // tiny helper shown below
	c.JSON(200, gin.H{"ok": ok})
}

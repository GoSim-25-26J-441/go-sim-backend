package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func Graphviz(c *gin.Context, upstreamURL string) {

	jobID := c.Param("id")

	spec, err := fetchJSON(
		fmt.Sprintf("%s/jobs/%s/export?format=json&download=false", upstreamURL, jobID),
		10*time.Second,
	)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"ok": false, "error": "export: " + err.Error()})
		return
	}

	dot := buildDOTFromSpec(spec)

	c.Header("Content-Type", "text/vnd.graphviz; charset=utf-8")
	c.String(http.StatusOK, dot)
}

func buildDOTFromSpec(spec map[string]any) string {

	services := tryArrayNames(spec, "services", "name")
	deps := tryDeps(spec)

	var b strings.Builder
	b.WriteString("digraph G {\n")
	b.WriteString("  rankdir=LR;\n")

	for _, s := range services {
		fmt.Fprintf(&b, "  \"%s\" [shape=box];\n", s)
	}

	for _, d := range deps {
		from, to, kind := d[0], d[1], d[2]
		if kind == "" {
			kind = "call"
		}
		label := strings.ToUpper(kind)
		fmt.Fprintf(&b, "  \"%s\" -> \"%s\" [label=\"%s\"];\n", from, to, label)
	}

	b.WriteString("}\n")
	return b.String()
}

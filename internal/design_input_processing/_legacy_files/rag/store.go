package rag

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var docs []Doc

func Load(dir string) error {
	docs = docs[:0]
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext != ".md" && ext != ".txt" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		docs = append(docs, Doc{ID: path, Title: title, Content: string(b)})
		return nil
	})
	return err
}

func Search(q string) []Result {
	q = strings.ToLower(q)
	type scored struct {
		res  Result
		hits int
	}
	out := make([]scored, 0, len(docs))
	for _, d := range docs {
		text := strings.ToLower(d.Title + "\n" + d.Content)
		hits := 0
		for _, tok := range strings.Fields(q) {
			if tok == "" {
				continue
			}
			if strings.Contains(text, tok) {
				hits++
			}
		}
		if hits > 0 {
			snip := d.Content
			if len(snip) > 240 {
				snip = snip[:240] + "..."
			}
			out = append(out, scored{Result{ID: d.ID, Title: d.Title, Snippet: snip, Score: float64(hits)}, hits})
		}
	}
	// naive sort by hits
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].hits > out[i].hits {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	res := make([]Result, len(out))
	for i, s := range out {
		res[i] = s.res
	}
	return res
}

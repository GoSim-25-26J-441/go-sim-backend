package rag

type Doc struct {
	ID      string
	Title   string
	Content string
}

type Result struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

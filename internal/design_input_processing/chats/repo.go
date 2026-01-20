package chats

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

type projectInfo struct {
	ID                      string
	CurrentDiagramVersionID *string
}

func (r *Repo) getProject(ctx context.Context, userID, publicID string) (*projectInfo, error) {
	const q = `
select
  id::text,
  case when current_diagram_version_id is null then null else current_diagram_version_id::text end
from projects
where public_id=$1 and user_id=$2 and deleted_at is null
`
	var p projectInfo
	if err := r.db.QueryRow(ctx, q, publicID, userID).Scan(&p.ID, &p.CurrentDiagramVersionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *Repo) CreateThread(ctx context.Context, userID, projectPublicID string, title *string, bindingMode string) (*Thread, error) {
	if bindingMode == "" {
		bindingMode = BindingFollowLatest
	}

	p, err := r.getProject(ctx, userID, projectPublicID)
	if err != nil {
		return nil, err
	}

	const q = `
insert into chat_threads (project_id, title, binding_mode)
values ($1, $2, $3)
returning id::text, created_at
`
	var id string
	var created time.Time
	if err := r.db.QueryRow(ctx, q, p.ID, title, bindingMode).Scan(&id, &created); err != nil {
		return nil, err
	}

	return &Thread{
		ID:              id,
		ProjectID:       p.ID,
		ProjectPublicID: projectPublicID,
		Title:           title,
		BindingMode:     bindingMode,
		CreatedAt:       created,
	}, nil
}

func (r *Repo) ListThreads(ctx context.Context, userID, projectPublicID string) ([]Thread, error) {
	p, err := r.getProject(ctx, userID, projectPublicID)
	if err != nil {
		return nil, err
	}

	const q = `
select
  id::text,
  title,
  binding_mode,
  case when pinned_diagram_version_id is null then null else pinned_diagram_version_id::text end,
  created_at
from chat_threads
where project_id=$1
order by created_at desc
`
	rows, err := r.db.Query(ctx, q, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Thread{}
	for rows.Next() {
		var t Thread
		t.ProjectID = p.ID
		t.ProjectPublicID = projectPublicID
		if err := rows.Scan(&t.ID, &t.Title, &t.BindingMode, &t.PinnedDiagramVersionID, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

type threadInfo struct {
	ID                     string
	ProjectID              string
	BindingMode            string
	PinnedDiagramVersionID *string
}

func (r *Repo) getThread(ctx context.Context, projectID, threadID string) (*threadInfo, error) {
	const q = `
select
  id::text,
  project_id::text,
  binding_mode,
  case when pinned_diagram_version_id is null then null else pinned_diagram_version_id::text end
from chat_threads
where id=$1 and project_id=$2
`
	var t threadInfo
	if err := r.db.QueryRow(ctx, q, threadID, projectID).Scan(&t.ID, &t.ProjectID, &t.BindingMode, &t.PinnedDiagramVersionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repo) ResolveDiagramContext(ctx context.Context, projectID, userID, projectPublicID, threadID string) (*string, json.RawMessage, error) {
	p, err := r.getProject(ctx, userID, projectPublicID)
	if err != nil {
		return nil, nil, err
	}

	t, err := r.getThread(ctx, p.ID, threadID)
	if err != nil {
		return nil, nil, err
	}

	var use *string
	if t.BindingMode == BindingPinned {
		use = t.PinnedDiagramVersionID
	} else {
		use = p.CurrentDiagramVersionID
	}

	if use == nil || *use == "" {
		return nil, json.RawMessage(`{}`), nil
	}

	const q = `
select
  coalesce(spec_summary, '{}'::jsonb)::text,
  coalesce(diagram_json, '{}'::jsonb)::text
from diagram_versions
where id=$1
`
	var specText string
	var diagramText string
	if err := r.db.QueryRow(ctx, q, *use).Scan(&specText, &diagramText); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	spec := json.RawMessage(specText)
	// if spec_summary is empty object, use diagram_json for now
	if len(spec) == 0 || string(spec) == "{}" {
		spec = json.RawMessage(diagramText)
	}
	return use, spec, nil
}

func (r *Repo) ListHistoryForUIGP(ctx context.Context, projectID, threadID string, limit int) ([]string, []string, error) {
	if limit <= 0 {
		limit = 20
	}

	const q = `
select role, content
from chat_messages
where thread_id=$1 and project_id=$2
order by created_at desc
limit $3
`
	rows, err := r.db.Query(ctx, q, threadID, projectID, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var roles []string
	var contents []string
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, nil, err
		}
		roles = append(roles, role)
		contents = append(contents, content)
	}
	return roles, contents, nil
}

type InsertAttachment struct {
	ObjectKey string
	MimeType  *string
	FileName  *string
	FileSize  *int64
	Width     *int
	Height    *int
}

func (r *Repo) InsertTurn(ctx context.Context, userID, projectPublicID, threadID string,
	userContent string, assistantContent string, assistantSource *string, assistantRefs []string,
	diagramVersionIDUsed *string,
	userAttachments []InsertAttachment,
) (*Message, *Message, error) {

	p, err := r.getProject(ctx, userID, projectPublicID)
	if err != nil {
		return nil, nil, err
	}
	_, err = r.getThread(ctx, p.ID, threadID)
	if err != nil {
		return nil, nil, err
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const insMsg = `
insert into chat_messages (thread_id, project_id, role, content, diagram_version_id_used)
values ($1, $2, $3, $4, $5)
returning id::text, created_at
`
	var userMsg Message
	userMsg.ThreadID = threadID
	userMsg.ProjectID = p.ID
	userMsg.Role = "user"
	userMsg.Content = userContent
	userMsg.DiagramVersionIDUsed = diagramVersionIDUsed

	if err := tx.QueryRow(ctx, insMsg, threadID, p.ID, "user", userContent, diagramVersionIDUsed).Scan(&userMsg.ID, &userMsg.CreatedAt); err != nil {
		return nil, nil, err
	}

	if len(userAttachments) > 0 {
		const insAtt = `
insert into chat_message_attachments
(message_id, kind, object_key, mime_type, file_name, file_size_bytes, width, height)
values ($1, $2, $3, $4, $5, $6, $7, $8)
returning id::text, created_at
`
		for _, a := range userAttachments {
			kind := "image"
			var att Attachment
			att.Kind = kind
			att.ObjectKey = a.ObjectKey
			att.MimeType = a.MimeType
			att.FileName = a.FileName
			att.FileSizeBytes = a.FileSize
			att.Width = a.Width
			att.Height = a.Height

			if err := tx.QueryRow(ctx, insAtt, userMsg.ID, kind, a.ObjectKey, a.MimeType, a.FileName, a.FileSize, a.Width, a.Height).
				Scan(&att.ID, &att.CreatedAt); err != nil {
				return nil, nil, err
			}
			userMsg.Attachments = append(userMsg.Attachments, att)
		}
	}

	refsJSON, _ := json.Marshal(assistantRefs)
	refsStr := string(refsJSON)

	const insAsst = `
insert into chat_messages (thread_id, project_id, role, content, source, refs, diagram_version_id_used)
values ($1, $2, $3, $4, $5, $6::jsonb, $7)
returning id::text, created_at
`
	var asstMsg Message
	asstMsg.ThreadID = threadID
	asstMsg.ProjectID = p.ID
	asstMsg.Role = "assistant"
	asstMsg.Content = assistantContent
	asstMsg.Source = assistantSource
	asstMsg.Refs = assistantRefs
	asstMsg.DiagramVersionIDUsed = diagramVersionIDUsed

	if err := tx.QueryRow(ctx, insAsst, threadID, p.ID, "assistant", assistantContent, assistantSource, refsStr, diagramVersionIDUsed).
		Scan(&asstMsg.ID, &asstMsg.CreatedAt); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	return &userMsg, &asstMsg, nil
}

func (r *Repo) ListMessages(ctx context.Context, userID, projectPublicID, threadID string, limit int) ([]Message, error) {
	p, err := r.getProject(ctx, userID, projectPublicID)
	if err != nil {
		return nil, err
	}
	_, err = r.getThread(ctx, p.ID, threadID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}

	const q = `
select
  m.id::text,
  m.thread_id::text,
  m.role,
  m.content,
  m.source,
  coalesce(m.refs, '[]'::jsonb)::text,
  case when m.diagram_version_id_used is null then null else m.diagram_version_id_used::text end,
  m.created_at,

  a.id::text,
  a.kind,
  a.object_key,
  a.mime_type,
  a.file_name,
  a.file_size_bytes,
  a.width,
  a.height,
  a.created_at
from chat_messages m
left join chat_message_attachments a on a.message_id = m.id
where m.thread_id=$1 and m.project_id=$2
order by m.created_at asc
limit $3
`
	rows, err := r.db.Query(ctx, q, threadID, p.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rowMsg struct {
		msg Message
	}
	byID := map[string]*Message{}
	order := []string{}

	for rows.Next() {
		var (
			mID, tID, role, content string
			source                  *string
			refsText                string
			dvUsed                  *string
			created                 time.Time

			aID      *string
			aKind    *string
			aKey     *string
			aMime    *string
			aName    *string
			aSize    *int64
			aW       *int
			aH       *int
			aCreated *time.Time
		)

		if err := rows.Scan(
			&mID, &tID, &role, &content, &source, &refsText, &dvUsed, &created,
			&aID, &aKind, &aKey, &aMime, &aName, &aSize, &aW, &aH, &aCreated,
		); err != nil {
			return nil, err
		}

		m, ok := byID[mID]
		if !ok {
			var refs []string
			_ = json.Unmarshal([]byte(refsText), &refs)

			newM := &Message{
				ID:                   mID,
				ThreadID:             tID,
				ProjectID:            p.ID,
				Role:                 role,
				Content:              content,
				Source:               source,
				Refs:                 refs,
				DiagramVersionIDUsed: dvUsed,
				CreatedAt:            created,
				Attachments:          []Attachment{},
			}
			byID[mID] = newM
			order = append(order, mID)
			m = newM
		}

		if aID != nil && aKey != nil && aCreated != nil && aKind != nil {
			m.Attachments = append(m.Attachments, Attachment{
				ID:            *aID,
				Kind:          *aKind,
				ObjectKey:     *aKey,
				MimeType:      aMime,
				FileName:      aName,
				FileSizeBytes: aSize,
				Width:         aW,
				Height:        aH,
				CreatedAt:     *aCreated,
			})
		}
	}

	out := make([]Message, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

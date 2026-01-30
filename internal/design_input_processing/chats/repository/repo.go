package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/chats/domain"
)

type Repo struct {
	db *sql.DB
}

func New(db *sql.DB) *Repo {
	return &Repo{db: db}
}

type projectInfo struct {
	PublicID                string
	CurrentDiagramVersionID *string
}

func (r *Repo) getProject(ctx context.Context, userFirebaseUID, publicID string) (*projectInfo, error) {
	const q = `
select
  public_id,
  current_diagram_version_id
from projects
where public_id=$1
  and user_firebase_uid=$2
  and deleted_at is null
`
	var p projectInfo
	if err := r.db.QueryRowContext(ctx, q, publicID, userFirebaseUID).Scan(&p.PublicID, &p.CurrentDiagramVersionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

type threadInfo struct {
	ID                     string
	ProjectPublicID        string
	UserFirebaseUID        string
	BindingMode            string
	PinnedDiagramVersionID *string
}

func (r *Repo) getThread(ctx context.Context, userFirebaseUID, projectPublicID, threadID string) (*threadInfo, error) {
	const q = `
select
  id,
  project_public_id,
  user_firebase_uid,
  binding_mode,
  pinned_diagram_version_id
from chat_threads
where id=$1
  and project_public_id=$2
  and user_firebase_uid=$3
`
	var t threadInfo
	if err := r.db.QueryRowContext(ctx, q, threadID, projectPublicID, userFirebaseUID).
		Scan(&t.ID, &t.ProjectPublicID, &t.UserFirebaseUID, &t.BindingMode, &t.PinnedDiagramVersionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repo) CreateThread(ctx context.Context, userFirebaseUID, projectPublicID string, title *string, bindingMode string) (*domain.Thread, error) {
	if bindingMode == "" {
		bindingMode = domain.BindingFollowLatest
	}

	// ensure project belongs to user
	if _, err := r.getProject(ctx, userFirebaseUID, projectPublicID); err != nil {
		return nil, err
	}

	id, err := domain.NewID("thr")
	if err != nil {
		return nil, err
	}

	const q = `
insert into chat_threads (id, project_public_id, user_firebase_uid, title, binding_mode)
values ($1, $2, $3, $4, $5)
returning id, created_at
`
	var created time.Time
	if err := r.db.QueryRowContext(ctx, q, id, projectPublicID, userFirebaseUID, title, bindingMode).Scan(&id, &created); err != nil {
		return nil, err
	}

	return &domain.Thread{
		ID:              id,
		ProjectPublicID: projectPublicID,
		Title:           title,
		BindingMode:     bindingMode,
		CreatedAt:       created,
	}, nil
}

func (r *Repo) ListThreads(ctx context.Context, userFirebaseUID, projectPublicID string) ([]domain.Thread, error) {
	// ensure project belongs to user
	if _, err := r.getProject(ctx, userFirebaseUID, projectPublicID); err != nil {
		return nil, err
	}

	const q = `
select
  id,
  title,
  binding_mode,
  pinned_diagram_version_id,
  created_at
from chat_threads
where project_public_id=$1
  and user_firebase_uid=$2
order by created_at desc
`
	rows, err := r.db.QueryContext(ctx, q, projectPublicID, userFirebaseUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Thread{}
	for rows.Next() {
		var t domain.Thread
		t.ProjectPublicID = projectPublicID
		if err := rows.Scan(&t.ID, &t.Title, &t.BindingMode, &t.PinnedDiagramVersionID, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repo) ResolveDiagramContext(ctx context.Context, userFirebaseUID, projectPublicID, threadID string) (*string, json.RawMessage, error) {
	p, err := r.getProject(ctx, userFirebaseUID, projectPublicID)
	if err != nil {
		return nil, nil, err
	}
	t, err := r.getThread(ctx, userFirebaseUID, projectPublicID, threadID)
	if err != nil {
		return nil, nil, err
	}

	var use *string
	if t.BindingMode == domain.BindingPinned {
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
  and project_public_id=$2
  and user_firebase_uid=$3
`
	var specText, diagramText string
	if err := r.db.QueryRowContext(ctx, q, *use, projectPublicID, userFirebaseUID).Scan(&specText, &diagramText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, err
	}

	spec := json.RawMessage(specText)
	if len(spec) == 0 || string(spec) == "{}" {
		spec = json.RawMessage(diagramText)
	}

	return use, spec, nil
}

func (r *Repo) ListHistoryForUIGP(ctx context.Context, userFirebaseUID, projectPublicID, threadID string, limit int) ([]string, []string, error) {
	if limit <= 0 {
		limit = 20
	}

	// ensure thread belongs to user/project
	if _, err := r.getThread(ctx, userFirebaseUID, projectPublicID, threadID); err != nil {
		return nil, nil, err
	}

	const q = `
select role, content
from chat_messages
where thread_id=$1
  and project_public_id=$2
  and user_firebase_uid=$3
order by created_at desc
limit $4
`
	rows, err := r.db.QueryContext(ctx, q, threadID, projectPublicID, userFirebaseUID, limit)
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
	if err := rows.Err(); err != nil {
		return nil, nil, err
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

func (r *Repo) InsertTurn(
	ctx context.Context,
	userFirebaseUID, projectPublicID, threadID string,
	userContent string,
	assistantContent string,
	assistantSource *string,
	assistantRefs []string,
	diagramVersionIDUsed *string,
	userAttachments []InsertAttachment,
) (*domain.Message, *domain.Message, error) {

	// validate ownership
	if _, err := r.getProject(ctx, userFirebaseUID, projectPublicID); err != nil {
		return nil, nil, err
	}
	if _, err := r.getThread(ctx, userFirebaseUID, projectPublicID, threadID); err != nil {
		return nil, nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	userMsgID, err := domain.NewID("msg")
	if err != nil {
		return nil, nil, err
	}

	const insMsg = `
insert into chat_messages
(id, thread_id, project_public_id, user_firebase_uid, role, content, diagram_version_id_used)
values ($1, $2, $3, $4, $5, $6, $7)
returning id, created_at
`
	var userMsg domain.Message
	userMsg.ThreadID = threadID
	userMsg.ProjectID = "" // not used in new schema
	userMsg.Role = "user"
	userMsg.Content = userContent
	userMsg.DiagramVersionIDUsed = diagramVersionIDUsed

	if err := tx.QueryRowContext(ctx, insMsg,
		userMsgID, threadID, projectPublicID, userFirebaseUID,
		"user", userContent, diagramVersionIDUsed,
	).Scan(&userMsg.ID, &userMsg.CreatedAt); err != nil {
		return nil, nil, err
	}

	// attachments
	if len(userAttachments) > 0 {
		const insAtt = `
insert into chat_message_attachments
(id, message_id, kind, object_key, mime_type, file_name, file_size_bytes, width, height)
values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
returning id, created_at
`
		for _, a := range userAttachments {
			if a.ObjectKey == "" {
				continue
			}
			attID, err := domain.NewID("att")
			if err != nil {
				return nil, nil, err
			}

			kind := "image"
			var att domain.Attachment
			att.Kind = kind
			att.ObjectKey = a.ObjectKey
			att.MimeType = a.MimeType
			att.FileName = a.FileName
			att.FileSizeBytes = a.FileSize
			att.Width = a.Width
			att.Height = a.Height

			if err := tx.QueryRowContext(ctx, insAtt,
				attID, userMsg.ID, kind, a.ObjectKey, a.MimeType, a.FileName, a.FileSize, a.Width, a.Height,
			).Scan(&att.ID, &att.CreatedAt); err != nil {
				return nil, nil, err
			}

			userMsg.Attachments = append(userMsg.Attachments, att)
		}
	}

	// assistant message
	asstMsgID, err := domain.NewID("msg")
	if err != nil {
		return nil, nil, err
	}

	refsJSON, _ := json.Marshal(assistantRefs)

	const insAsst = `
insert into chat_messages
(id, thread_id, project_public_id, user_firebase_uid, role, content, source, refs, diagram_version_id_used)
values ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9)
returning id, created_at
`
	var asstMsg domain.Message
	asstMsg.ThreadID = threadID
	asstMsg.Role = "assistant"
	asstMsg.Content = assistantContent
	asstMsg.Source = assistantSource
	asstMsg.Refs = assistantRefs
	asstMsg.DiagramVersionIDUsed = diagramVersionIDUsed

	if err := tx.QueryRowContext(ctx, insAsst,
		asstMsgID, threadID, projectPublicID, userFirebaseUID,
		"assistant", assistantContent, assistantSource, string(refsJSON), diagramVersionIDUsed,
	).Scan(&asstMsg.ID, &asstMsg.CreatedAt); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return &userMsg, &asstMsg, nil
}

func (r *Repo) ListMessages(ctx context.Context, userFirebaseUID, projectPublicID, threadID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 50
	}

	// validate ownership
	if _, err := r.getThread(ctx, userFirebaseUID, projectPublicID, threadID); err != nil {
		return nil, err
	}

	const q = `
select
  m.id,
  m.thread_id,
  m.role,
  m.content,
  m.source,
  coalesce(m.refs, '[]'::jsonb)::text,
  m.diagram_version_id_used,
  m.created_at,

  a.id,
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
where m.thread_id=$1
  and m.project_public_id=$2
  and m.user_firebase_uid=$3
order by m.created_at asc
limit $4
`
	rows, err := r.db.QueryContext(ctx, q, threadID, projectPublicID, userFirebaseUID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[string]*domain.Message{}
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

			newM := &domain.Message{
				ID:                   mID,
				ThreadID:             tID,
				Role:                 role,
				Content:              content,
				Source:               source,
				Refs:                 refs,
				DiagramVersionIDUsed: dvUsed,
				CreatedAt:            created,
				Attachments:          []domain.Attachment{},
			}
			byID[mID] = newM
			order = append(order, mID)
			m = newM
		}

		if aID != nil && aKey != nil && aCreated != nil && aKind != nil {
			m.Attachments = append(m.Attachments, domain.Attachment{
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

	out := make([]domain.Message, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

package routes

import (
	"database/sql"

	fbauth "firebase.google.com/go/v4/auth"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/auth"
	authhttp "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/http"
	authmiddleware "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/middleware"
	authrepo "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/repository"
	authservice "github.com/GoSim-25-26J-441/go-sim-backend/internal/auth/service"

	diproutes "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/api/http/routes"
	dipllm "github.com/GoSim-25-26J-441/go-sim-backend/internal/design_input_processing/llm"

	"github.com/GoSim-25-26J-441/go-sim-backend/internal/projects"
	"github.com/GoSim-25-26J-441/go-sim-backend/internal/users"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type V1Deps struct {
	DBPool   *pgxpool.Pool
	AuthSQL  *sql.DB
	Firebase *fbauth.Client

	UIGP *dipllm.UIGPClient
}

func RegisterV1(r *gin.Engine, dep V1Deps) {
	api := r.Group("/api/v1")

	if dep.Firebase != nil && dep.AuthSQL != nil {
		authGroup := api.Group("/auth")
		authGroup.Use(authmiddleware.FirebaseAuthMiddleware(dep.Firebase))

		repo := authrepo.NewUserRepository(dep.AuthSQL)
		svc := authservice.NewAuthService(repo)
		h := authhttp.New(svc)
		h.Register(authGroup)
	}

	protected := api.Group("")
	userRepo := users.NewRepo(dep.DBPool)
	protected.Use(auth.WithUser(userRepo))

	projectsGroup := protected.Group("/projects")
	projectRepo := projects.NewRepo(dep.DBPool)
	projects.Register(projectsGroup, projectRepo)

	diproutes.RegisterProjectRoutes(projectsGroup, diproutes.ProjectDeps{
		DB:   dep.DBPool,
		UIGP: dep.UIGP,
	})
}

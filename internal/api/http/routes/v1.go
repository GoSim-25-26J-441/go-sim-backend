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

	"github.com/gin-gonic/gin"
)

type V1Deps struct {
	DB      *sql.DB
	Firebase *fbauth.Client

	UIGP *dipllm.UIGPClient
}

func RegisterV1(api *gin.RouterGroup, dep V1Deps) {

	if dep.Firebase != nil && dep.DB != nil {
		authGroup := api.Group("/auth")
		authGroup.Use(authmiddleware.FirebaseAuthMiddleware(dep.Firebase))

		repo := authrepo.NewUserRepository(dep.DB)
		svc := authservice.NewAuthService(repo)
		h := authhttp.New(svc)
		h.Register(authGroup)
	}

	protected := api.Group("")
	userRepo := authrepo.NewUserRepository(dep.DB)
	protected.Use(auth.WithUser(userRepo))

	projectsGroup := protected.Group("/projects")
	projectRepo := projects.NewRepo(dep.DB)
	projects.Register(projectsGroup, projectRepo)

	diproutes.RegisterProjectRoutes(projectsGroup, diproutes.ProjectDeps{
		DB:   dep.DB,
		UIGP: dep.UIGP,
	})
}

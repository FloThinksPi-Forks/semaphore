package db

import (
	"errors"
	log "github.com/Sirupsen/logrus"
	"github.com/go-gorp/gorp/v3"
	"time"
)



const databaseTimeFormat = "2006-01-02T15:04:05:99Z"

// GetParsedTime returns the timestamp as it will retrieved from the database
// This allows us to create timestamp consistency on return values from create requests
func GetParsedTime(t time.Time) time.Time {
	parsedTime, err := time.Parse(databaseTimeFormat, t.Format(databaseTimeFormat))
	if err != nil {
		log.Error(err)
	}
	return parsedTime
}

type RetrieveQueryParams struct {
	Offset       int
	Count        int
	SortBy       string
	SortInverted bool
}

var ErrNotFound = errors.New("sql: no rows in result set")
var ErrInvalidOperation = errors.New("sql: no rows in result set")
var Forbidden = errors.New("sql: no rows in result set")

type Store interface {
	Connect() error
	Close() error
	Migrate() error

	GetEnvironment(projectID int, environmentID int) (Environment, error)
	GetEnvironments(projectID int, params RetrieveQueryParams) ([]Environment, error)
	UpdateEnvironment(env Environment) error
	CreateEnvironment(env Environment) (Environment, error)
	DeleteEnvironment(projectID int, templateID int) error
	DeleteEnvironmentSoft(projectID int, templateID int) error

	GetInventory(projectID int, inventoryID int) (Inventory, error)
	GetInventories(projectID int, params RetrieveQueryParams) ([]Inventory, error)
	UpdateInventory(inventory Inventory) error
	CreateInventory(inventory Inventory) (Inventory, error)
	DeleteInventory(projectID int, inventoryID int) error
	DeleteInventorySoft(projectID int, inventoryID int) error

	GetRepository(projectID int, repositoryID int) (Repository, error)
	GetRepositories(projectID int, params RetrieveQueryParams) ([]Repository, error)
	UpdateRepository(repository Repository) error
	CreateRepository(repository Repository) (Repository, error)
	DeleteRepository(projectID int, repositoryID int) error
	DeleteRepositorySoft(projectID int, repositoryID int) error

	GetAccessKey(projectID int, accessKeyID int) (AccessKey, error)
	GetAccessKeys(projectID int, params RetrieveQueryParams) ([]AccessKey, error)
	UpdateAccessKey(accessKey AccessKey) error
	CreateAccessKey(accessKey AccessKey) (AccessKey, error)
	DeleteAccessKey(projectID int, accessKeyID int) error
	DeleteAccessKeySoft(projectID int, accessKeyID int) error

	GetGlobalAccessKey(accessKeyID int) (AccessKey, error)
	GetGlobalAccessKeys(params RetrieveQueryParams) ([]AccessKey, error)
	UpdateGlobalAccessKey(accessKey AccessKey) error
	CreateGlobalAccessKey(accessKey AccessKey) (AccessKey, error)
	DeleteGlobalAccessKey(accessKeyID int) error
	DeleteGlobalAccessKeySoft(accessKeyID int) error

	GetUsers(params RetrieveQueryParams) ([]User, error)
	CreateUser(user User) (User, error)
	DeleteUser(userID int) error
	UpdateUser(user User) error
	SetUserPassword(userID int, password string) error
	GetUser(userID int) (User, error)
	GetUserByLoginOrEmail(login string, email string) (User, error)

	GetProject(projectID int) (Project, error)
	GetProjects(userID int) ([]Project, error)
	CreateProject(project Project) (Project, error)
	DeleteProject(projectID int) error
	UpdateProject(project Project) error

	GetTemplates(projectID int, params RetrieveQueryParams) ([]Template, error)
	CreateTemplate(template Template) (Template, error)
	UpdateTemplate(template Template) error
	GetTemplate(projectID int, templateID int) (Template, error)
	DeleteTemplate(projectID int, templateID int) error

	GetProjectUsers(projectID int, params RetrieveQueryParams) ([]User, error)
	CreateProjectUser(projectUser ProjectUser) (ProjectUser, error)
	DeleteProjectUser(projectID int, userID int) error
	GetProjectUser(projectID int, userID int) (ProjectUser, error)
	UpdateProjectUser(projectUser ProjectUser) error

	CreateEvent(event Event) (Event, error)
	GetUserEvents(userID int, params RetrieveQueryParams) ([]Event, error)
	GetEvents(projectID int, params RetrieveQueryParams) ([]Event, error)

	GetAPITokens(userID int) ([]APIToken, error)
	CreateAPIToken(token APIToken) (APIToken, error)
	GetAPIToken(tokenID string) (APIToken, error)
	ExpireAPIToken(userID int, tokenID string) error

	GetSession(userID int, sessionID int) (Session, error)
	CreateSession(session Session) (Session, error)
	ExpireSession(userID int, sessionID int) error
	TouchSession(userID int, sessionID int) error

	CreateTask(task Task) (Task, error)
	UpdateTask(task Task) error

	GetTemplateTasks(projectID int, templateID int, params RetrieveQueryParams) ([]TaskWithTpl, error)
	GetProjectTasks(projectID int, params RetrieveQueryParams) ([]TaskWithTpl, error)
	GetTask(projectID int, taskID int) (Task, error)
	DeleteTaskWithOutputs(projectID int, taskID int) error
	GetTaskOutputs(projectID int, taskID int) ([]TaskOutput, error)
	CreateTaskOutput(output TaskOutput) (TaskOutput, error)


	Sql() *gorp.DbMap
}



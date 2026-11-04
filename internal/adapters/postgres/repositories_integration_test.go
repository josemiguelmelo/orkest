package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/task"
)

func TestPostgresRepositories(t *testing.T) {
	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		if os.Getenv("DATABASE_URL") == "" && os.Getenv("POSTGRES_HOST") == "" && os.Getenv("POSTGRES_PORT") == "" && os.Getenv("POSTGRES_USERNAME") == "" && os.Getenv("POSTGRES_PASSWORD") == "" && os.Getenv("POSTGRES_DB") == "" {
			t.Skip("PostgreSQL connection settings are required for repository integration tests")
		}
		t.Fatal(err)
	}
	ctx := context.Background()
	database, err := initialize(ctx, dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL database: %v", err)
	}
	defer database.Close()

	userRepository := NewUserPostgresRepository(database)
	organizationRepository := NewOrganizationPostgresRepository(database)
	workspaceRepository := NewWorkspacePostgresRepository(database)

	ownerSubject, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	memberSubject, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := userRepository.RegisterUser(ctx, ownerSubject, "repo-owner", "owner@example.test")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	defer deleteTestUser(t, database, owner.ID)
	ownerAgain, err := userRepository.RegisterUser(ctx, ownerSubject, "renamed-owner", "changed@example.test")
	if err != nil {
		t.Fatalf("register owner again: %v", err)
	}
	if ownerAgain.ID != owner.ID || ownerAgain.Username != owner.Username || ownerAgain.Email != owner.Email {
		t.Fatalf("registration did not preserve the first profile: first=%+v second=%+v", owner, ownerAgain)
	}

	member, err := userRepository.RegisterUser(ctx, memberSubject, "repo-member", "member@example.test")
	if err != nil {
		t.Fatalf("register member: %v", err)
	}
	defer deleteTestUser(t, database, member.ID)

	organizationID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	orphanedOrganizationID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = organizationRepository.CreateOrganization(ctx, orphanedOrganizationID, "missing-user", "should roll back"); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("expected missing owner to fail organization creation, got %v", err)
	}
	var orphanCount int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE id=$1`, orphanedOrganizationID).Scan(&orphanCount); err != nil {
		t.Fatalf("check failed organization creation: %v", err)
	}
	if orphanCount != 0 {
		t.Fatal("organization insert was not rolled back after owner membership failed")
	}
	org, err := organizationRepository.CreateOrganization(ctx, organizationID, owner.ID, "repository integration")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	defer func() {
		_, _ = database.pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, org.ID)
	}()
	role, err := organizationRepository.OrganizationRole(ctx, owner.ID, org.ID)
	if err != nil || role != "owner" {
		t.Fatalf("organization creation should atomically grant owner role; role=%q err=%v", role, err)
	}
	if err = organizationRepository.SetOrganizationMember(ctx, owner.ID, org.ID, owner.ID, "member"); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("expected last-owner conflict, got %v", err)
	}
	if err = organizationRepository.SetOrganizationMember(ctx, owner.ID, org.ID, member.ID, "member"); err != nil {
		t.Fatalf("add organization member: %v", err)
	}

	workspaceID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = workspaceRepository.CreateWorkspace(ctx, workspaceID, org.ID, "repository workspace"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err = workspaceRepository.SetWorkspaceMember(ctx, org.ID, workspaceID, member.ID, "viewer"); err != nil {
		t.Fatalf("add workspace member: %v", err)
	}
	visible, err := workspaceRepository.Workspaces(ctx, org.ID, member.ID)
	if err != nil || len(visible) != 1 || visible[0].ID != workspaceID {
		t.Fatalf("workspace membership was not visible: workspaces=%+v err=%v", visible, err)
	}
	isolated, err := workspaceRepository.Workspaces(ctx, "another-organization", member.ID)
	if err != nil || len(isolated) != 0 {
		t.Fatalf("workspace query crossed organization boundary: workspaces=%+v err=%v", isolated, err)
	}

	codeRepositoryID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	codeRepositoryRepository := NewCodeReposPostgresRepository(database)
	_, err = database.pool.Exec(ctx, `INSERT INTO code_repositories (id,workspace_id,provider_name,url) VALUES ($1,$2,'github',$3)`, codeRepositoryID, workspaceID, "https://github.com/acme/project.git")
	if err != nil {
		t.Fatalf("insert legacy public repository fixture: %v", err)
	}
	codeRepository, err := codeRepositoryRepository.CodeRepository(ctx, org.ID, workspaceID, codeRepositoryID)
	if err != nil || codeRepository.ID != codeRepositoryID || codeRepository.WorkspaceID != workspaceID || codeRepository.ProviderName != "github" {
		t.Fatalf("create code repository: repository=%+v err=%v", codeRepository, err)
	}
	codeRepositories, err := codeRepositoryRepository.CodeRepositories(ctx, org.ID, workspaceID)
	if err != nil || len(codeRepositories) != 1 || codeRepositories[0] != codeRepository {
		t.Fatalf("list code repositories: repositories=%+v err=%v", codeRepositories, err)
	}
	if _, err = codeRepositoryRepository.CodeRepository(ctx, "another-organization", workspaceID, codeRepositoryID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("code repository read crossed organization boundary: %v", err)
	}
	integrationID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	integration, err := codeRepositoryRepository.CreateCodingIntegration(ctx, coderepository.CodingIntegration{ID: integrationID, WorkspaceID: workspaceID, Name: "GitHub", ProviderName: "github", ProviderConfig: []byte(`{}`)}, "encrypted-token")
	if err != nil {
		t.Fatalf("create coding integration: %v", err)
	}
	attached, err := codeRepositoryRepository.CreateIntegratedCodeRepository(ctx, coderepository.CodeRepository{ID: "replacement-id", WorkspaceID: workspaceID, ProviderName: "github", URL: codeRepository.URL, IntegrationID: integration.ID, ProviderRepositoryID: "42", DefaultBranch: "trunk", Name: "acme/project", Private: true})
	if err != nil || attached.ID != codeRepositoryID || attached.IntegrationID != integrationID || attached.DefaultBranch != "trunk" || !attached.Private {
		t.Fatalf("attach matching repository: item=%+v err=%v", attached, err)
	}
	if err = codeRepositoryRepository.DeleteCodingIntegration(ctx, workspaceID, integrationID); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("delete referenced coding integration error=%v", err)
	}
	if _, err = codeRepositoryRepository.CreateIntegratedCodeRepository(ctx, coderepository.CodeRepository{ID: "duplicate-repository", WorkspaceID: workspaceID, ProviderName: "github", URL: "https://github.com/acme/project-renamed.git", IntegrationID: integrationID, ProviderRepositoryID: "42", DefaultBranch: "trunk"}); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("duplicate provider repository identity error=%v", err)
	}

	agentID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	agentRepository := NewAgentPostgresRepository(database, nil)
	worker, err := agentRepository.CreateAgent(ctx, agentID, workspaceID, agent.AgentDefinition{Name: "worker", Runtime: agent.RuntimeOpenCode, Provider: "test", Model: "test-model", PermissionPolicy: agent.PermissionPolicyAsk})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	taskRepository := NewTaskPostgresRepository(database)
	taskID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	createdTask, err := createWorkflowTask(ctx, taskRepository, taskID, workspaceID, task.TaskInput{WorkflowID: workspaceID, Title: "Add endpoint", Description: "Task body", Status: "todo", AssigneeAgentID: worker.ID, Labels: []string{"api"}, RepositoryIDs: []string{codeRepositoryID}})
	if err != nil || createdTask.SourceProvider != "manual" || createdTask.AssigneeAgentID != worker.ID || len(createdTask.Repositories) != 1 || createdTask.Repositories[0].ID != codeRepositoryID {
		t.Fatalf("create task: task=%+v err=%v", createdTask, err)
	}
	if err = codeRepositoryRepository.DeleteCodeRepository(ctx, org.ID, workspaceID, codeRepositoryID); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("expected task-referenced repository deletion conflict, got %v", err)
	}
	t.Run("bounded task listing", func(t *testing.T) { testTaskListingUsesBoundedScopedQueries(t, database, org.ID, workspaceID, taskID) })
	t.Run("workflow delivery claims", func(t *testing.T) { testWorkflowDeliveryClaims(t, database, org.ID, workspaceID, owner.ID, worker.ID) })
	t.Run("workspace workflow transitions", func(t *testing.T) { testWorkspaceWorkflow(t, database, org.ID, owner.ID, worker.ID) })
	t.Run("task artifacts", func(t *testing.T) { testTaskArtifacts(t, database, org.ID, workspaceID, taskID, owner.ID) })
	t.Run("task sessions and prompt delivery", func(t *testing.T) { testTaskSessions(t, database, org.ID, workspaceID, taskID) })
	commentID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	comment, err := taskRepository.CreateComment(ctx, commentID, workspaceID, taskID, member.ID, "Starting work")
	if err != nil || comment.AuthorUserID != member.ID {
		t.Fatalf("create task comment: comment=%+v err=%v", comment, err)
	}
	comments, err := taskRepository.Comments(ctx, org.ID, workspaceID, taskID)
	if err != nil || len(comments) != 1 || comments[0].Body != "Starting work" {
		t.Fatalf("list task comments: comments=%+v err=%v", comments, err)
	}
	if _, err = taskRepository.Task(ctx, "another-organization", workspaceID, taskID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("task read crossed organization boundary: %v", err)
	}
	if err = agentRepository.DeleteAgent(ctx, org.ID, workspaceID, worker.ID); err != nil {
		t.Fatalf("delete assigned agent: %v", err)
	}
	unassignedTask, err := taskRepository.Task(ctx, org.ID, workspaceID, taskID)
	if err != nil || unassignedTask.AssigneeAgentID != "" {
		t.Fatalf("deleted agent assignment was not cleared: task=%+v err=%v", unassignedTask, err)
	}

	// Exercise generated row mappings and affected-row handling through the repositories.
	updatedOrg, err := organizationRepository.UpdateOrganization(ctx, org.ID, owner.ID, "updated organization")
	if err != nil || updatedOrg.Name != "updated organization" || updatedOrg.Role != "owner" {
		t.Fatalf("update organization: organization=%+v err=%v", updatedOrg, err)
	}
	fetchedOrg, err := organizationRepository.Organization(ctx, member.ID, org.ID)
	if err != nil || fetchedOrg.Name != updatedOrg.Name || fetchedOrg.Role != "member" {
		t.Fatalf("get organization: organization=%+v err=%v", fetchedOrg, err)
	}
	orgs, err := organizationRepository.Organizations(ctx, member.ID)
	if err != nil || len(orgs) != 1 || orgs[0] != fetchedOrg {
		t.Fatalf("list organizations: organizations=%+v err=%v", orgs, err)
	}
	members, err := organizationRepository.OrganizationMembers(ctx, org.ID)
	if err != nil || len(members) != 2 || members[0].User.ID != member.ID || members[0].Role != "member" {
		t.Fatalf("list organization members: members=%+v err=%v", members, err)
	}
	inherited, err := workspaceRepository.Workspace(ctx, org.ID, workspaceID, owner.ID)
	if err != nil || inherited.Role != "owner" {
		t.Fatalf("inherited workspace access: workspace=%+v err=%v", inherited, err)
	}
	if _, err = workspaceRepository.Workspace(ctx, "another-organization", workspaceID, owner.ID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("workspace read crossed organization boundary: %v", err)
	}
	t.Run("execution profiles", func(t *testing.T) { testExecutionProfiles(t, database, workspaceID) })
	updatedWorkspace, err := workspaceRepository.UpdateWorkspace(ctx, org.ID, workspaceID, "updated workspace")
	if err != nil || updatedWorkspace.Name != "updated workspace" || updatedWorkspace.OrganizationID != org.ID {
		t.Fatalf("update workspace: workspace=%+v err=%v", updatedWorkspace, err)
	}
	workspaceMembers, err := workspaceRepository.WorkspaceMembers(ctx, org.ID, workspaceID)
	if err != nil || len(workspaceMembers) != 1 || workspaceMembers[0].User.ID != member.ID || workspaceMembers[0].Role != "viewer" {
		t.Fatalf("list workspace members: members=%+v err=%v", workspaceMembers, err)
	}
	if err = workspaceRepository.RemoveWorkspaceMember(ctx, "another-organization", workspaceID, member.ID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("workspace membership removal crossed organization boundary: %v", err)
	}
	if err = workspaceRepository.RemoveWorkspaceMember(ctx, org.ID, workspaceID, member.ID); err != nil {
		t.Fatalf("remove workspace member: %v", err)
	}
	if _, err = workspaceRepository.Workspace(ctx, org.ID, workspaceID, member.ID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("removed workspace member retained access: %v", err)
	}
	if err = organizationRepository.RemoveOrganizationMember(ctx, owner.ID, org.ID, owner.ID); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("expected last-owner removal conflict: %v", err)
	}
	if err = organizationRepository.RemoveOrganizationMember(ctx, owner.ID, org.ID, member.ID); err != nil {
		t.Fatalf("remove organization member: %v", err)
	}
	if err = workspaceRepository.DeleteWorkspace(ctx, org.ID, workspaceID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if _, err = codeRepositoryRepository.CodeRepository(ctx, org.ID, workspaceID, codeRepositoryID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("code repository survived workspace deletion: %v", err)
	}
	if err = workspaceRepository.DeleteWorkspace(ctx, org.ID, workspaceID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("expected missing workspace on repeated delete: %v", err)
	}
	if err = organizationRepository.DeleteOrganization(ctx, org.ID); err != nil {
		t.Fatalf("delete organization: %v", err)
	}
	if err = organizationRepository.DeleteOrganization(ctx, org.ID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("expected missing organization on repeated delete: %v", err)
	}
}

func deleteTestUser(t *testing.T, database *PostgresDatabase, userID string) {
	t.Helper()
	if _, err := database.pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID); err != nil {
		t.Errorf("delete integration user: %v", err)
	}
}

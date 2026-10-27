export type ClientOptions = {
    baseUrl: `${string}://${string}` | (string & {});
};
export type SkillInput = {
    /**
     * Complete SKILL.md including required frontmatter
     */
    skillMD: string;
};
export type Skill = {
    id: string;
    scope: 'global' | 'organization' | 'workspace';
    organizationId?: string;
    workspaceId?: string;
    skillMD: string;
    createdAt: string;
    updatedAt: string;
};
export type Skills = Array<Skill>;
export type CurrentUser = {
    user: User;
    organizations: Array<Organization>;
    readonly platformAdmin: boolean;
};
export type User = {
    id: string;
    username: string;
    email: string;
};
export type Organization = {
    id: string;
    name: string;
    role?: 'owner' | 'admin' | 'member';
};
export type Workspace = {
    id: string;
    organizationId: string;
    name: string;
    role?: 'owner' | 'admin' | 'member' | 'viewer';
};
export type WorkspaceEnvironmentInput = {
    value: string;
    secret?: boolean;
};
export type WorkspaceEnvironmentValue = {
    name: string;
    secret: boolean;
    /**
     * Omitted for secrets
     */
    value?: string;
};
export type Membership = {
    user: User;
    role: 'owner' | 'admin' | 'member' | 'viewer';
};
export type TaskInput = {
    /**
     * Explicitly start another work step after a failed run; active execution conflicts.
     */
    startWork?: boolean;
    /**
     * Session choice when explicitly starting work. Defaults to continue when a session exists.
     */
    sessionMode?: 'continue' | 'new';
    /**
     * Optional initial prompt for a user-started work step.
     */
    promptText?: string;
    commandId?: string;
    title: string;
    description?: string;
    status?: 'todo' | 'in_progress' | 'in_review' | 'done';
    assigneeAgentId?: string;
    labels?: Array<string>;
    componentIds?: Array<string>;
};
export type Task = {
    id: string;
    workspaceId: string;
    title: string;
    description: string;
    sourceProvider: string;
    sourceId?: string;
    /**
     * Source link, possibly empty for draft items.
     */
    sourceUrl?: string;
    status: 'todo' | 'in_progress' | 'in_review' | 'done';
    assigneeAgentId?: string;
    labels: Array<string>;
    components: Array<Component>;
    createdAt: string;
    updatedAt: string;
};
export type TaskComment = {
    id: string;
    workspaceId: string;
    taskId: string;
    authorUserId: string;
    body: string;
    createdAt: string;
};
export type TaskRun = {
    recoveryBlocked?: boolean;
    executionState?: string;
    queuePaused?: boolean;
    queueDepth?: number;
    runId: string;
    taskSessionId?: string;
    taskId: string;
    status: string;
    failureStage?: string;
    failureReason?: string;
    lastSequence: number;
    updatedAt: string;
};
export type TaskRunEvent = {
    runId: string;
    sequence: number;
    payload: {
        [key: string]: unknown;
    };
};
export type ComponentInput = {
    identifier: string;
    name: string;
    repositoryId: string;
    description?: string;
};
export type Component = {
    id: string;
    workspaceId: string;
    identifier: string;
    name: string;
    repositoryId: string;
    description: string;
    createdAt: string;
    updatedAt: string;
};
export type Agent = {
    id: string;
    workspaceId: string;
    name: string;
    runtime: 'opencode';
    provider: string;
    model: string;
    description: string;
    systemPrompt: string;
    mcpServerIds: Array<string>;
    skillIds: Array<string>;
};
export type McpServer = {
    id: string;
    workspaceId: string;
    name: string;
    url: string;
    hasHeaders: boolean;
};
export type CodeRepository = {
    id: string;
    workspaceId: string;
    providerName: 'github';
    url: string;
};
export type CodeRepositoryCreate = {
    providerName: 'github';
    url: string;
};
export type TaskIntegrationInput = {
    provider: string;
    name: string;
    enabled?: boolean;
    config: TaskIntegrationConfig;
    labels?: Array<string>;
    interval?: string;
};
export type TaskIntegration = {
    id: string;
    workspaceId: string;
    provider: string;
    name: string;
    enabled: boolean;
    config: TaskIntegrationConfig;
    labels: Array<string>;
    interval: string;
    lastSynced?: string;
    lastError?: string;
    createdAt: string;
    updatedAt: string;
};
/**
 * Provider-defined config. Retrieve allowed fields and constraints from GET /api/task-integration-providers.
 */
export type TaskIntegrationConfig = {
    [key: string]: unknown;
};
export type TaskIntegrationProvider = {
    provider: string;
    /**
     * JSON Schema that validates the provider-specific TaskIntegrationInput config.
     */
    configSchema: {
        [key: string]: unknown;
    };
};
export type Error = {
    error: string;
};
export type OperationStatus = {
    status: string;
};
export type CurrentUserWritable = {
    user: User;
    organizations: Array<Organization>;
};
export type TaskIntegrationInputWritable = {
    provider: string;
    name: string;
    enabled?: boolean;
    config: TaskIntegrationConfig;
    labels?: Array<string>;
    interval?: string;
    credential?: string;
};
export type NamedResource = {
    name: string;
};
export type OrganizationRole = {
    role: 'owner' | 'admin' | 'member';
};
export type WorkspaceRole = {
    role: 'admin' | 'member' | 'viewer';
};
export type AgentDefinition = {
    name: string;
    runtime: 'opencode';
    provider: string;
    model: string;
    description: string;
    systemPrompt: string;
    mcpServerIds?: Array<string>;
    skillIds?: Array<string>;
};
export type McpServerInput = {
    name: string;
    url: string;
    headers?: {
        [key: string]: string;
    };
};
export type CodeRepositoryCreate2 = CodeRepositoryCreate;
export type TaskIntegration2 = TaskIntegrationInputWritable;
export type GetHealthData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/healthz';
};
export type GetHealthResponses = {
    /**
     * HTTP process is running
     */
    200: {
        status: 'ok';
    };
};
export type GetHealthResponse = GetHealthResponses[keyof GetHealthResponses];
export type StartLoginData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/auth/login';
};
export type CompleteLoginData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/auth/callback';
};
export type CompleteLoginErrors = {
    /**
     * Invalid login state
     */
    400: unknown;
    /**
     * Token exchange or validation failed
     */
    401: unknown;
};
export type LogoutData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/auth/logout';
};
export type LogoutResponses = {
    /**
     * Session cleared
     */
    204: void;
};
export type LogoutResponse = LogoutResponses[keyof LogoutResponses];
export type GetCurrentUserData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/api/me';
};
export type GetCurrentUserErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetCurrentUserError = GetCurrentUserErrors[keyof GetCurrentUserErrors];
export type GetCurrentUserResponses = {
    /**
     * Internal user profile and organization memberships
     */
    200: CurrentUser;
};
export type GetCurrentUserResponse = GetCurrentUserResponses[keyof GetCurrentUserResponses];
export type ListOrganizationsData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/api/organizations';
};
export type ListOrganizationsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListOrganizationsError = ListOrganizationsErrors[keyof ListOrganizationsErrors];
export type ListOrganizationsResponses = {
    /**
     * Organizations the user belongs to
     */
    200: Array<Organization>;
};
export type ListOrganizationsResponse = ListOrganizationsResponses[keyof ListOrganizationsResponses];
export type CreateOrganizationData = {
    body: NamedResource;
    path?: never;
    query?: never;
    url: '/api/organizations';
};
export type CreateOrganizationErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateOrganizationError = CreateOrganizationErrors[keyof CreateOrganizationErrors];
export type CreateOrganizationResponses = {
    /**
     * Organization created; creator is Owner
     */
    201: Organization;
};
export type CreateOrganizationResponse = CreateOrganizationResponses[keyof CreateOrganizationResponses];
export type DeleteOrganizationData = {
    body?: never;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}';
};
export type DeleteOrganizationErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteOrganizationError = DeleteOrganizationErrors[keyof DeleteOrganizationErrors];
export type DeleteOrganizationResponses = {
    /**
     * Organization deleted
     */
    204: void;
};
export type DeleteOrganizationResponse = DeleteOrganizationResponses[keyof DeleteOrganizationResponses];
export type GetOrganizationData = {
    body?: never;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}';
};
export type GetOrganizationErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetOrganizationError = GetOrganizationErrors[keyof GetOrganizationErrors];
export type GetOrganizationResponses = {
    /**
     * Organization
     */
    200: Organization;
};
export type GetOrganizationResponse = GetOrganizationResponses[keyof GetOrganizationResponses];
export type UpdateOrganizationData = {
    body: NamedResource;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}';
};
export type UpdateOrganizationErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type UpdateOrganizationError = UpdateOrganizationErrors[keyof UpdateOrganizationErrors];
export type UpdateOrganizationResponses = {
    /**
     * Updated organization
     */
    200: Organization;
};
export type UpdateOrganizationResponse = UpdateOrganizationResponses[keyof UpdateOrganizationResponses];
export type ListOrganizationMembersData = {
    body?: never;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/members';
};
export type ListOrganizationMembersErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListOrganizationMembersError = ListOrganizationMembersErrors[keyof ListOrganizationMembersErrors];
export type ListOrganizationMembersResponses = {
    /**
     * Organization members
     */
    200: Array<Membership>;
};
export type ListOrganizationMembersResponse = ListOrganizationMembersResponses[keyof ListOrganizationMembersResponses];
export type RemoveOrganizationMemberData = {
    body?: never;
    path: {
        orgID: string;
        userID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/members/{userID}';
};
export type RemoveOrganizationMemberErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type RemoveOrganizationMemberError = RemoveOrganizationMemberErrors[keyof RemoveOrganizationMemberErrors];
export type RemoveOrganizationMemberResponses = {
    /**
     * Membership removed
     */
    204: void;
};
export type RemoveOrganizationMemberResponse = RemoveOrganizationMemberResponses[keyof RemoveOrganizationMemberResponses];
export type SetOrganizationMemberData = {
    body: OrganizationRole;
    path: {
        orgID: string;
        userID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/members/{userID}';
};
export type SetOrganizationMemberErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type SetOrganizationMemberError = SetOrganizationMemberErrors[keyof SetOrganizationMemberErrors];
export type SetOrganizationMemberResponses = {
    /**
     * Membership updated
     */
    200: OperationStatus;
};
export type SetOrganizationMemberResponse = SetOrganizationMemberResponses[keyof SetOrganizationMemberResponses];
export type ListWorkspacesData = {
    body?: never;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces';
};
export type ListWorkspacesErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListWorkspacesError = ListWorkspacesErrors[keyof ListWorkspacesErrors];
export type ListWorkspacesResponses = {
    /**
     * Workspaces accessible to the user
     */
    200: Array<Workspace>;
};
export type ListWorkspacesResponse = ListWorkspacesResponses[keyof ListWorkspacesResponses];
export type CreateWorkspaceData = {
    body: NamedResource;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces';
};
export type CreateWorkspaceErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateWorkspaceError = CreateWorkspaceErrors[keyof CreateWorkspaceErrors];
export type CreateWorkspaceResponses = {
    /**
     * Workspace created
     */
    201: Workspace;
};
export type CreateWorkspaceResponse = CreateWorkspaceResponses[keyof CreateWorkspaceResponses];
export type DeleteWorkspaceData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}';
};
export type DeleteWorkspaceErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteWorkspaceError = DeleteWorkspaceErrors[keyof DeleteWorkspaceErrors];
export type DeleteWorkspaceResponses = {
    /**
     * Workspace deleted
     */
    204: void;
};
export type DeleteWorkspaceResponse = DeleteWorkspaceResponses[keyof DeleteWorkspaceResponses];
export type GetWorkspaceData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}';
};
export type GetWorkspaceErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetWorkspaceError = GetWorkspaceErrors[keyof GetWorkspaceErrors];
export type GetWorkspaceResponses = {
    /**
     * Workspace
     */
    200: Workspace;
};
export type GetWorkspaceResponse = GetWorkspaceResponses[keyof GetWorkspaceResponses];
export type UpdateWorkspaceData = {
    body: NamedResource;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}';
};
export type UpdateWorkspaceErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type UpdateWorkspaceError = UpdateWorkspaceErrors[keyof UpdateWorkspaceErrors];
export type UpdateWorkspaceResponses = {
    /**
     * Updated workspace
     */
    200: Workspace;
};
export type UpdateWorkspaceResponse = UpdateWorkspaceResponses[keyof UpdateWorkspaceResponses];
export type ListWorkspaceMembersData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/members';
};
export type ListWorkspaceMembersErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListWorkspaceMembersError = ListWorkspaceMembersErrors[keyof ListWorkspaceMembersErrors];
export type ListWorkspaceMembersResponses = {
    /**
     * Workspace members
     */
    200: Array<Membership>;
};
export type ListWorkspaceMembersResponse = ListWorkspaceMembersResponses[keyof ListWorkspaceMembersResponses];
export type ListWorkspaceEnvironmentData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/environment';
};
export type ListWorkspaceEnvironmentErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListWorkspaceEnvironmentError = ListWorkspaceEnvironmentErrors[keyof ListWorkspaceEnvironmentErrors];
export type ListWorkspaceEnvironmentResponses = {
    /**
     * Variables are returned with values; secrets return only their names and secret status
     */
    200: Array<WorkspaceEnvironmentValue>;
};
export type ListWorkspaceEnvironmentResponse = ListWorkspaceEnvironmentResponses[keyof ListWorkspaceEnvironmentResponses];
export type DeleteWorkspaceEnvironmentData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        name: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/environment/{name}';
};
export type DeleteWorkspaceEnvironmentErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteWorkspaceEnvironmentError = DeleteWorkspaceEnvironmentErrors[keyof DeleteWorkspaceEnvironmentErrors];
export type DeleteWorkspaceEnvironmentResponses = {
    /**
     * Value deleted
     */
    204: void;
};
export type DeleteWorkspaceEnvironmentResponse = DeleteWorkspaceEnvironmentResponses[keyof DeleteWorkspaceEnvironmentResponses];
export type PutWorkspaceEnvironmentData = {
    body: WorkspaceEnvironmentInput;
    path: {
        orgID: string;
        workspaceID: string;
        name: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/environment/{name}';
};
export type PutWorkspaceEnvironmentErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
    /**
     * Secret encryption key is unavailable
     */
    503: Error;
};
export type PutWorkspaceEnvironmentError = PutWorkspaceEnvironmentErrors[keyof PutWorkspaceEnvironmentErrors];
export type PutWorkspaceEnvironmentResponses = {
    /**
     * Value updated
     */
    200: OperationStatus;
};
export type PutWorkspaceEnvironmentResponse = PutWorkspaceEnvironmentResponses[keyof PutWorkspaceEnvironmentResponses];
export type RemoveWorkspaceMemberData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        userID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/members/{userID}';
};
export type RemoveWorkspaceMemberErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type RemoveWorkspaceMemberError = RemoveWorkspaceMemberErrors[keyof RemoveWorkspaceMemberErrors];
export type RemoveWorkspaceMemberResponses = {
    /**
     * Membership removed
     */
    204: void;
};
export type RemoveWorkspaceMemberResponse = RemoveWorkspaceMemberResponses[keyof RemoveWorkspaceMemberResponses];
export type SetWorkspaceMemberData = {
    body: WorkspaceRole;
    path: {
        orgID: string;
        workspaceID: string;
        userID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/members/{userID}';
};
export type SetWorkspaceMemberErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type SetWorkspaceMemberError = SetWorkspaceMemberErrors[keyof SetWorkspaceMemberErrors];
export type SetWorkspaceMemberResponses = {
    /**
     * Membership updated
     */
    200: OperationStatus;
};
export type SetWorkspaceMemberResponse = SetWorkspaceMemberResponses[keyof SetWorkspaceMemberResponses];
export type ListCodeRepositoriesData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/code-repositories';
};
export type ListCodeRepositoriesErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListCodeRepositoriesError = ListCodeRepositoriesErrors[keyof ListCodeRepositoriesErrors];
export type ListCodeRepositoriesResponses = {
    /**
     * Workspace code repositories
     */
    200: Array<CodeRepository>;
};
export type ListCodeRepositoriesResponse = ListCodeRepositoriesResponses[keyof ListCodeRepositoriesResponses];
export type CreateCodeRepositoryData = {
    body: CodeRepositoryCreate2;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/code-repositories';
};
export type CreateCodeRepositoryErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateCodeRepositoryError = CreateCodeRepositoryErrors[keyof CreateCodeRepositoryErrors];
export type CreateCodeRepositoryResponses = {
    /**
     * Code repository registered
     */
    201: CodeRepository;
};
export type CreateCodeRepositoryResponse = CreateCodeRepositoryResponses[keyof CreateCodeRepositoryResponses];
export type DeleteCodeRepositoryData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        repositoryID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/code-repositories/{repositoryID}';
};
export type DeleteCodeRepositoryErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteCodeRepositoryError = DeleteCodeRepositoryErrors[keyof DeleteCodeRepositoryErrors];
export type DeleteCodeRepositoryResponses = {
    /**
     * Code repository removed
     */
    204: void;
};
export type DeleteCodeRepositoryResponse = DeleteCodeRepositoryResponses[keyof DeleteCodeRepositoryResponses];
export type GetCodeRepositoryData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        repositoryID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/code-repositories/{repositoryID}';
};
export type GetCodeRepositoryErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetCodeRepositoryError = GetCodeRepositoryErrors[keyof GetCodeRepositoryErrors];
export type GetCodeRepositoryResponses = {
    /**
     * Registered code repository
     */
    200: CodeRepository;
};
export type GetCodeRepositoryResponse = GetCodeRepositoryResponses[keyof GetCodeRepositoryResponses];
export type CloneCodeRepositoryData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        repositoryID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/code-repositories/{repositoryID}/clone';
};
export type CloneCodeRepositoryErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CloneCodeRepositoryError = CloneCodeRepositoryErrors[keyof CloneCodeRepositoryErrors];
export type CloneCodeRepositoryResponses = {
    /**
     * Repository cloned
     */
    204: void;
};
export type CloneCodeRepositoryResponse = CloneCodeRepositoryResponses[keyof CloneCodeRepositoryResponses];
export type SyncCodeRepositoryMainData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        repositoryID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/code-repositories/{repositoryID}/sync-main';
};
export type SyncCodeRepositoryMainErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type SyncCodeRepositoryMainError = SyncCodeRepositoryMainErrors[keyof SyncCodeRepositoryMainErrors];
export type SyncCodeRepositoryMainResponses = {
    /**
     * Checkout reset to the latest remote main branch
     */
    204: void;
};
export type SyncCodeRepositoryMainResponse = SyncCodeRepositoryMainResponses[keyof SyncCodeRepositoryMainResponses];
export type ListTaskIntegrationsData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/task-integrations';
};
export type ListTaskIntegrationsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListTaskIntegrationsError = ListTaskIntegrationsErrors[keyof ListTaskIntegrationsErrors];
export type ListTaskIntegrationsResponses = {
    /**
     * Configured task integrations
     */
    200: Array<TaskIntegration>;
};
export type ListTaskIntegrationsResponse = ListTaskIntegrationsResponses[keyof ListTaskIntegrationsResponses];
export type CreateTaskIntegrationData = {
    body: TaskIntegration2;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/task-integrations';
};
export type CreateTaskIntegrationErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
    /**
     * Provider connectivity or credential validation failed
     */
    502: Error;
    /**
     * Integration credential encryption is not configured
     */
    503: Error;
};
export type CreateTaskIntegrationError = CreateTaskIntegrationErrors[keyof CreateTaskIntegrationErrors];
export type CreateTaskIntegrationResponses = {
    /**
     * Integration configured
     */
    201: TaskIntegration;
};
export type CreateTaskIntegrationResponse = CreateTaskIntegrationResponses[keyof CreateTaskIntegrationResponses];
export type ListTaskIntegrationProvidersData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/api/task-integration-providers';
};
export type ListTaskIntegrationProvidersErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListTaskIntegrationProvidersError = ListTaskIntegrationProvidersErrors[keyof ListTaskIntegrationProvidersErrors];
export type ListTaskIntegrationProvidersResponses = {
    /**
     * Available providers and their config schemas
     */
    200: Array<TaskIntegrationProvider>;
};
export type ListTaskIntegrationProvidersResponse = ListTaskIntegrationProvidersResponses[keyof ListTaskIntegrationProvidersResponses];
export type ValidateTaskIntegrationData = {
    body: TaskIntegration2;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/task-integrations/validate';
};
export type ValidateTaskIntegrationErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
    /**
     * Provider connectivity or credential validation failed
     */
    502: Error;
};
export type ValidateTaskIntegrationError = ValidateTaskIntegrationErrors[keyof ValidateTaskIntegrationErrors];
export type ValidateTaskIntegrationResponses = {
    /**
     * Connectivity validated
     */
    200: OperationStatus;
};
export type ValidateTaskIntegrationResponse = ValidateTaskIntegrationResponses[keyof ValidateTaskIntegrationResponses];
export type DeleteTaskIntegrationData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        integrationID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/task-integrations/{integrationID}';
};
export type DeleteTaskIntegrationErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteTaskIntegrationError = DeleteTaskIntegrationErrors[keyof DeleteTaskIntegrationErrors];
export type DeleteTaskIntegrationResponses = {
    /**
     * Integration deleted
     */
    204: void;
};
export type DeleteTaskIntegrationResponse = DeleteTaskIntegrationResponses[keyof DeleteTaskIntegrationResponses];
export type UpdateTaskIntegrationData = {
    body: TaskIntegration2;
    path: {
        orgID: string;
        workspaceID: string;
        integrationID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/task-integrations/{integrationID}';
};
export type UpdateTaskIntegrationErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
    /**
     * Provider connectivity or credential validation failed
     */
    502: Error;
    /**
     * Integration credential encryption is not configured
     */
    503: Error;
};
export type UpdateTaskIntegrationError = UpdateTaskIntegrationErrors[keyof UpdateTaskIntegrationErrors];
export type UpdateTaskIntegrationResponses = {
    /**
     * Integration updated
     */
    200: TaskIntegration;
};
export type UpdateTaskIntegrationResponse = UpdateTaskIntegrationResponses[keyof UpdateTaskIntegrationResponses];
export type SyncTaskIntegrationNowData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        integrationID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/task-integrations/{integrationID}/sync';
};
export type SyncTaskIntegrationNowErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type SyncTaskIntegrationNowError = SyncTaskIntegrationNowErrors[keyof SyncTaskIntegrationNowErrors];
export type SyncTaskIntegrationNowResponses = {
    /**
     * Synchronization completed
     */
    204: void;
};
export type SyncTaskIntegrationNowResponse = SyncTaskIntegrationNowResponses[keyof SyncTaskIntegrationNowResponses];
export type ListMcpServersData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/mcp-servers';
};
export type ListMcpServersErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListMcpServersError = ListMcpServersErrors[keyof ListMcpServersErrors];
export type ListMcpServersResponses = {
    /**
     * Workspace MCP servers
     */
    200: Array<McpServer>;
};
export type ListMcpServersResponse = ListMcpServersResponses[keyof ListMcpServersResponses];
export type CreateMcpServerData = {
    body: McpServerInput;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/mcp-servers';
};
export type CreateMcpServerErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
    /**
     * MCP credential encryption is not configured
     */
    503: Error;
};
export type CreateMcpServerError = CreateMcpServerErrors[keyof CreateMcpServerErrors];
export type CreateMcpServerResponses = {
    /**
     * MCP server created
     */
    201: McpServer;
};
export type CreateMcpServerResponse = CreateMcpServerResponses[keyof CreateMcpServerResponses];
export type DeleteMcpServerData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        serverID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/mcp-servers/{serverID}';
};
export type DeleteMcpServerErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteMcpServerError = DeleteMcpServerErrors[keyof DeleteMcpServerErrors];
export type DeleteMcpServerResponses = {
    /**
     * MCP server deleted and removed from agent selections
     */
    204: void;
};
export type DeleteMcpServerResponse = DeleteMcpServerResponses[keyof DeleteMcpServerResponses];
export type UpdateMcpServerData = {
    body: McpServerInput;
    path: {
        orgID: string;
        workspaceID: string;
        serverID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/mcp-servers/{serverID}';
};
export type UpdateMcpServerErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
    /**
     * MCP credential encryption is not configured
     */
    503: Error;
};
export type UpdateMcpServerError = UpdateMcpServerErrors[keyof UpdateMcpServerErrors];
export type UpdateMcpServerResponses = {
    /**
     * MCP server updated; omitted headers preserve existing credentials
     */
    200: McpServer;
};
export type UpdateMcpServerResponse = UpdateMcpServerResponses[keyof UpdateMcpServerResponses];
export type ListAgentsData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/agents';
};
export type ListAgentsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListAgentsError = ListAgentsErrors[keyof ListAgentsErrors];
export type ListAgentsResponses = {
    /**
     * Workspace agents
     */
    200: Array<Agent>;
};
export type ListAgentsResponse = ListAgentsResponses[keyof ListAgentsResponses];
export type CreateAgentData = {
    body: AgentDefinition;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/agents';
};
export type CreateAgentErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateAgentError = CreateAgentErrors[keyof CreateAgentErrors];
export type CreateAgentResponses = {
    /**
     * Agent created
     */
    201: Agent;
};
export type CreateAgentResponse = CreateAgentResponses[keyof CreateAgentResponses];
export type DeleteAgentData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        agentID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/agents/{agentID}';
};
export type DeleteAgentErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteAgentError = DeleteAgentErrors[keyof DeleteAgentErrors];
export type DeleteAgentResponses = {
    /**
     * Agent deleted
     */
    204: void;
};
export type DeleteAgentResponse = DeleteAgentResponses[keyof DeleteAgentResponses];
export type GetAgentData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        agentID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/agents/{agentID}';
};
export type GetAgentErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetAgentError = GetAgentErrors[keyof GetAgentErrors];
export type GetAgentResponses = {
    /**
     * Agent definition
     */
    200: Agent;
};
export type GetAgentResponse = GetAgentResponses[keyof GetAgentResponses];
export type UpdateAgentData = {
    body: AgentDefinition;
    path: {
        orgID: string;
        workspaceID: string;
        agentID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/agents/{agentID}';
};
export type UpdateAgentErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type UpdateAgentError = UpdateAgentErrors[keyof UpdateAgentErrors];
export type UpdateAgentResponses = {
    /**
     * Agent updated
     */
    200: Agent;
};
export type UpdateAgentResponse = UpdateAgentResponses[keyof UpdateAgentResponses];
export type ListTasksData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks';
};
export type ListTasksErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListTasksError = ListTasksErrors[keyof ListTasksErrors];
export type ListTasksResponses = {
    /**
     * Workspace tasks
     */
    200: Array<Task>;
};
export type ListTasksResponse = ListTasksResponses[keyof ListTasksResponses];
export type CreateTaskData = {
    body: TaskInput;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks';
};
export type CreateTaskErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateTaskError = CreateTaskErrors[keyof CreateTaskErrors];
export type CreateTaskResponses = {
    /**
     * Created task
     */
    201: Task;
};
export type CreateTaskResponse = CreateTaskResponses[keyof CreateTaskResponses];
export type GetTaskData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}';
};
export type GetTaskErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetTaskError = GetTaskErrors[keyof GetTaskErrors];
export type GetTaskResponses = {
    /**
     * Task
     */
    200: Task;
};
export type GetTaskResponse = GetTaskResponses[keyof GetTaskResponses];
export type UpdateTaskData = {
    body: TaskInput;
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}';
};
export type UpdateTaskErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
    /**
     * Internal server error
     */
    500: Error;
    /**
     * Task was moved to in_progress but agent dispatch failed
     */
    502: Error;
};
export type UpdateTaskError = UpdateTaskErrors[keyof UpdateTaskErrors];
export type UpdateTaskResponses = {
    /**
     * Updated task
     */
    200: Task;
};
export type UpdateTaskResponse = UpdateTaskResponses[keyof UpdateTaskResponses];
export type ListTaskRunsData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs';
};
export type ListTaskRunsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Run history unavailable
     */
    503: Error;
};
export type ListTaskRunsError = ListTaskRunsErrors[keyof ListTaskRunsErrors];
export type ListTaskRunsResponses = {
    /**
     * Task run history
     */
    200: Array<TaskRun>;
};
export type ListTaskRunsResponse = ListTaskRunsResponses[keyof ListTaskRunsResponses];
export type ListTaskRunEventsData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
        runID: string;
    };
    query?: {
        afterSequence?: number;
    };
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs/{runID}/events';
};
export type ListTaskRunEventsErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
};
export type ListTaskRunEventsError = ListTaskRunEventsErrors[keyof ListTaskRunEventsErrors];
export type ListTaskRunEventsResponses = {
    /**
     * Ordered run activity
     */
    200: Array<TaskRunEvent>;
};
export type ListTaskRunEventsResponse = ListTaskRunEventsResponses[keyof ListTaskRunEventsResponses];
export type SendTaskRunPromptData = {
    body: {
        commandId: string;
        text: string;
    };
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
        runID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs/{runID}/prompts';
};
export type SendTaskRunPromptErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
};
export type SendTaskRunPromptError = SendTaskRunPromptErrors[keyof SendTaskRunPromptErrors];
export type SendTaskRunPromptResponses = {
    /**
     * Prompt accepted
     */
    202: {
        commandId: string;
    };
};
export type SendTaskRunPromptResponse = SendTaskRunPromptResponses[keyof SendTaskRunPromptResponses];
export type CancelTaskRunPromptData = {
    body: {
        commandId: string;
    };
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
        runID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs/{runID}/cancel';
};
export type CancelTaskRunPromptErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
};
export type CancelTaskRunPromptError = CancelTaskRunPromptErrors[keyof CancelTaskRunPromptErrors];
export type CancelTaskRunPromptResponses = {
    /**
     * Cancellation accepted
     */
    202: unknown;
};
export type RespondTaskRunPermissionData = {
    body: {
        requestId: string;
        sessionId: string;
        optionId: string;
    };
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
        runID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs/{runID}/permissions/respond';
};
export type RespondTaskRunPermissionErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
};
export type RespondTaskRunPermissionError = RespondTaskRunPermissionErrors[keyof RespondTaskRunPermissionErrors];
export type RespondTaskRunPermissionResponses = {
    /**
     * Permission response accepted
     */
    202: unknown;
};
export type ListTaskCommentsData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/comments';
};
export type ListTaskCommentsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListTaskCommentsError = ListTaskCommentsErrors[keyof ListTaskCommentsErrors];
export type ListTaskCommentsResponses = {
    /**
     * Task comments
     */
    200: Array<TaskComment>;
};
export type ListTaskCommentsResponse = ListTaskCommentsResponses[keyof ListTaskCommentsResponses];
export type CreateTaskCommentData = {
    body: {
        body: string;
    };
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/comments';
};
export type CreateTaskCommentErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateTaskCommentError = CreateTaskCommentErrors[keyof CreateTaskCommentErrors];
export type CreateTaskCommentResponses = {
    /**
     * Created comment
     */
    201: TaskComment;
};
export type CreateTaskCommentResponse = CreateTaskCommentResponses[keyof CreateTaskCommentResponses];
export type ListComponentsData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/components';
};
export type ListComponentsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListComponentsError = ListComponentsErrors[keyof ListComponentsErrors];
export type ListComponentsResponses = {
    /**
     * Workspace components
     */
    200: Array<Component>;
};
export type ListComponentsResponse = ListComponentsResponses[keyof ListComponentsResponses];
export type CreateComponentData = {
    body: ComponentInput;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/components';
};
export type CreateComponentErrors = {
    /**
     * Request body or role is invalid
     */
    400: Error;
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateComponentError = CreateComponentErrors[keyof CreateComponentErrors];
export type CreateComponentResponses = {
    /**
     * Created component
     */
    201: Component;
};
export type CreateComponentResponse = CreateComponentResponses[keyof CreateComponentResponses];
export type UpdateComponentData = {
    body: ComponentInput;
    path: {
        orgID: string;
        workspaceID: string;
        componentID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/components/{componentID}';
};
export type UpdateComponentErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Operation conflicts with membership invariants
     */
    409: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type UpdateComponentError = UpdateComponentErrors[keyof UpdateComponentErrors];
export type UpdateComponentResponses = {
    /**
     * Updated component
     */
    200: Component;
};
export type UpdateComponentResponse = UpdateComponentResponses[keyof UpdateComponentResponses];
export type ListGlobalSkillsData = {
    body?: never;
    path?: never;
    query?: never;
    url: '/api/skills';
};
export type ListGlobalSkillsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListGlobalSkillsError = ListGlobalSkillsErrors[keyof ListGlobalSkillsErrors];
export type ListGlobalSkillsResponses = {
    /**
     * Global skills
     */
    200: Skills;
};
export type ListGlobalSkillsResponse = ListGlobalSkillsResponses[keyof ListGlobalSkillsResponses];
export type CreateGlobalSkillData = {
    body: SkillInput;
    path?: never;
    query?: never;
    url: '/api/skills';
};
export type CreateGlobalSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateGlobalSkillError = CreateGlobalSkillErrors[keyof CreateGlobalSkillErrors];
export type CreateGlobalSkillResponses = {
    /**
     * Created skill
     */
    201: Skill;
};
export type CreateGlobalSkillResponse = CreateGlobalSkillResponses[keyof CreateGlobalSkillResponses];
export type DeleteGlobalSkillData = {
    body?: never;
    path: {
        skillID: string;
    };
    query?: never;
    url: '/api/skills/{skillID}';
};
export type DeleteGlobalSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteGlobalSkillError = DeleteGlobalSkillErrors[keyof DeleteGlobalSkillErrors];
export type DeleteGlobalSkillResponses = {
    /**
     * Deleted skill
     */
    204: void;
};
export type DeleteGlobalSkillResponse = DeleteGlobalSkillResponses[keyof DeleteGlobalSkillResponses];
export type GetGlobalSkillData = {
    body?: never;
    path: {
        skillID: string;
    };
    query?: never;
    url: '/api/skills/{skillID}';
};
export type GetGlobalSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetGlobalSkillError = GetGlobalSkillErrors[keyof GetGlobalSkillErrors];
export type GetGlobalSkillResponses = {
    /**
     * Skill
     */
    200: Skill;
};
export type GetGlobalSkillResponse = GetGlobalSkillResponses[keyof GetGlobalSkillResponses];
export type UpdateGlobalSkillData = {
    body: SkillInput;
    path: {
        skillID: string;
    };
    query?: never;
    url: '/api/skills/{skillID}';
};
export type UpdateGlobalSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type UpdateGlobalSkillError = UpdateGlobalSkillErrors[keyof UpdateGlobalSkillErrors];
export type UpdateGlobalSkillResponses = {
    /**
     * Updated skill
     */
    200: Skill;
};
export type UpdateGlobalSkillResponse = UpdateGlobalSkillResponses[keyof UpdateGlobalSkillResponses];
export type ListOrganizationSkillsData = {
    body?: never;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/skills';
};
export type ListOrganizationSkillsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListOrganizationSkillsError = ListOrganizationSkillsErrors[keyof ListOrganizationSkillsErrors];
export type ListOrganizationSkillsResponses = {
    /**
     * Organization skills
     */
    200: Skills;
};
export type ListOrganizationSkillsResponse = ListOrganizationSkillsResponses[keyof ListOrganizationSkillsResponses];
export type CreateOrganizationSkillData = {
    body: SkillInput;
    path: {
        orgID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/skills';
};
export type CreateOrganizationSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateOrganizationSkillError = CreateOrganizationSkillErrors[keyof CreateOrganizationSkillErrors];
export type CreateOrganizationSkillResponses = {
    /**
     * Created skill
     */
    201: Skill;
};
export type CreateOrganizationSkillResponse = CreateOrganizationSkillResponses[keyof CreateOrganizationSkillResponses];
export type DeleteOrganizationSkillData = {
    body?: never;
    path: {
        orgID: string;
        skillID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/skills/{skillID}';
};
export type DeleteOrganizationSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteOrganizationSkillError = DeleteOrganizationSkillErrors[keyof DeleteOrganizationSkillErrors];
export type DeleteOrganizationSkillResponses = {
    /**
     * Deleted skill
     */
    204: void;
};
export type DeleteOrganizationSkillResponse = DeleteOrganizationSkillResponses[keyof DeleteOrganizationSkillResponses];
export type GetOrganizationSkillData = {
    body?: never;
    path: {
        orgID: string;
        skillID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/skills/{skillID}';
};
export type GetOrganizationSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetOrganizationSkillError = GetOrganizationSkillErrors[keyof GetOrganizationSkillErrors];
export type GetOrganizationSkillResponses = {
    /**
     * Skill
     */
    200: Skill;
};
export type GetOrganizationSkillResponse = GetOrganizationSkillResponses[keyof GetOrganizationSkillResponses];
export type UpdateOrganizationSkillData = {
    body: SkillInput;
    path: {
        orgID: string;
        skillID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/skills/{skillID}';
};
export type UpdateOrganizationSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type UpdateOrganizationSkillError = UpdateOrganizationSkillErrors[keyof UpdateOrganizationSkillErrors];
export type UpdateOrganizationSkillResponses = {
    /**
     * Updated skill
     */
    200: Skill;
};
export type UpdateOrganizationSkillResponse = UpdateOrganizationSkillResponses[keyof UpdateOrganizationSkillResponses];
export type ListWorkspaceSkillsData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/skills';
};
export type ListWorkspaceSkillsErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type ListWorkspaceSkillsError = ListWorkspaceSkillsErrors[keyof ListWorkspaceSkillsErrors];
export type ListWorkspaceSkillsResponses = {
    /**
     * Skills visible in workspace
     */
    200: Skills;
};
export type ListWorkspaceSkillsResponse = ListWorkspaceSkillsResponses[keyof ListWorkspaceSkillsResponses];
export type CreateWorkspaceSkillData = {
    body: SkillInput;
    path: {
        orgID: string;
        workspaceID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/skills';
};
export type CreateWorkspaceSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type CreateWorkspaceSkillError = CreateWorkspaceSkillErrors[keyof CreateWorkspaceSkillErrors];
export type CreateWorkspaceSkillResponses = {
    /**
     * Created skill
     */
    201: Skill;
};
export type CreateWorkspaceSkillResponse = CreateWorkspaceSkillResponses[keyof CreateWorkspaceSkillResponses];
export type DeleteWorkspaceSkillData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        skillID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/skills/{skillID}';
};
export type DeleteWorkspaceSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type DeleteWorkspaceSkillError = DeleteWorkspaceSkillErrors[keyof DeleteWorkspaceSkillErrors];
export type DeleteWorkspaceSkillResponses = {
    /**
     * Deleted skill
     */
    204: void;
};
export type DeleteWorkspaceSkillResponse = DeleteWorkspaceSkillResponses[keyof DeleteWorkspaceSkillResponses];
export type GetWorkspaceSkillData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        skillID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/skills/{skillID}';
};
export type GetWorkspaceSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type GetWorkspaceSkillError = GetWorkspaceSkillErrors[keyof GetWorkspaceSkillErrors];
export type GetWorkspaceSkillResponses = {
    /**
     * Skill
     */
    200: Skill;
};
export type GetWorkspaceSkillResponse = GetWorkspaceSkillResponses[keyof GetWorkspaceSkillResponses];
export type UpdateWorkspaceSkillData = {
    body: SkillInput;
    path: {
        orgID: string;
        workspaceID: string;
        skillID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/skills/{skillID}';
};
export type UpdateWorkspaceSkillErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Internal server error
     */
    500: Error;
};
export type UpdateWorkspaceSkillError = UpdateWorkspaceSkillErrors[keyof UpdateWorkspaceSkillErrors];
export type UpdateWorkspaceSkillResponses = {
    /**
     * Updated skill
     */
    200: Skill;
};
export type UpdateWorkspaceSkillResponse = UpdateWorkspaceSkillResponses[keyof UpdateWorkspaceSkillResponses];
export type ResumeTaskRunQueueData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
        runID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs/{runID}/queue/resume';
};
export type ResumeTaskRunQueueErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Queue unavailable
     */
    409: Error;
};
export type ResumeTaskRunQueueError = ResumeTaskRunQueueErrors[keyof ResumeTaskRunQueueErrors];
export type ResumeTaskRunQueueResponses = {
    /**
     * Queue updated
     */
    200: unknown;
};
export type DiscardTaskRunQueueData = {
    body?: never;
    path: {
        orgID: string;
        workspaceID: string;
        taskID: string;
        runID: string;
    };
    query?: never;
    url: '/api/organizations/{orgID}/workspaces/{workspaceID}/tasks/{taskID}/runs/{runID}/queue/discard';
};
export type DiscardTaskRunQueueErrors = {
    /**
     * Missing or invalid credentials
     */
    401: Error;
    /**
     * Insufficient role permissions
     */
    403: Error;
    /**
     * Resource is absent or inaccessible
     */
    404: Error;
    /**
     * Queue unavailable
     */
    409: Error;
};
export type DiscardTaskRunQueueError = DiscardTaskRunQueueErrors[keyof DiscardTaskRunQueueErrors];
export type DiscardTaskRunQueueResponses = {
    /**
     * Queue updated
     */
    200: unknown;
};

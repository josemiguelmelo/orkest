import * as z from 'zod';
export declare const zSkillInput: z.ZodObject<{
    skillMD: z.ZodString;
}, z.core.$strip>;
export declare const zSkill: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zSkills: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zUser: z.ZodObject<{
    id: z.ZodString;
    username: z.ZodString;
    email: z.ZodString;
}, z.core.$strip>;
export declare const zOrganization: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
    }>>;
}, z.core.$strip>;
export declare const zCurrentUser: z.ZodObject<{
    user: z.ZodObject<{
        id: z.ZodString;
        username: z.ZodString;
        email: z.ZodString;
    }, z.core.$strip>;
    organizations: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        name: z.ZodString;
        role: z.ZodOptional<z.ZodEnum<{
            owner: "owner";
            admin: "admin";
            member: "member";
        }>>;
    }, z.core.$strip>>;
    platformAdmin: z.ZodReadonly<z.ZodBoolean>;
}, z.core.$strip>;
export declare const zWorkspace: z.ZodObject<{
    id: z.ZodString;
    organizationId: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>>;
}, z.core.$strip>;
export declare const zMembership: z.ZodObject<{
    user: z.ZodObject<{
        id: z.ZodString;
        username: z.ZodString;
        email: z.ZodString;
    }, z.core.$strip>;
    role: z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>;
}, z.core.$strip>;
export declare const zTaskInput: z.ZodObject<{
    title: z.ZodString;
    description: z.ZodOptional<z.ZodString>;
    status: z.ZodOptional<z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    componentIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
}, z.core.$strip>;
export declare const zTaskComment: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    taskId: z.ZodString;
    authorUserId: z.ZodString;
    body: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zTaskRun: z.ZodObject<{
    runId: z.ZodString;
    taskId: z.ZodString;
    status: z.ZodString;
    failureStage: z.ZodOptional<z.ZodString>;
    failureReason: z.ZodOptional<z.ZodString>;
    lastSequence: z.ZodInt;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zTaskRunEvent: z.ZodObject<{
    runId: z.ZodString;
    sequence: z.ZodInt;
    payload: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>;
export declare const zComponentInput: z.ZodObject<{
    identifier: z.ZodString;
    name: z.ZodString;
    repositoryId: z.ZodString;
    description: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zComponent: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    identifier: z.ZodString;
    name: z.ZodString;
    repositoryId: z.ZodString;
    description: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zTask: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    status: z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    components: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        workspaceId: z.ZodString;
        identifier: z.ZodString;
        name: z.ZodString;
        repositoryId: z.ZodString;
        description: z.ZodString;
        createdAt: z.ZodISODateTime;
        updatedAt: z.ZodISODateTime;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zAgent: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
}, z.core.$strip>;
export declare const zMcpServer: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    url: z.ZodURL;
    hasHeaders: z.ZodBoolean;
}, z.core.$strip>;
export declare const zCodeRepository: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
    }>;
    url: z.ZodURL;
}, z.core.$strip>;
export declare const zCodeRepositoryCreate: z.ZodObject<{
    providerName: z.ZodEnum<{
        github: "github";
    }>;
    url: z.ZodURL;
}, z.core.$strip>;
/**
 * Provider-defined config. Retrieve allowed fields and constraints from GET /api/task-integration-providers.
 */
export declare const zTaskIntegrationConfig: z.ZodRecord<z.ZodString, z.ZodUnknown>;
export declare const zTaskIntegrationInput: z.ZodObject<{
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodOptional<z.ZodBoolean>;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    interval: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zTaskIntegration: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodBoolean;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodArray<z.ZodString>;
    interval: z.ZodString;
    lastSynced: z.ZodOptional<z.ZodISODateTime>;
    lastError: z.ZodOptional<z.ZodString>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zTaskIntegrationProvider: z.ZodObject<{
    provider: z.ZodString;
    configSchema: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>;
export declare const zError: z.ZodObject<{
    error: z.ZodString;
}, z.core.$strip>;
export declare const zOperationStatus: z.ZodObject<{
    status: z.ZodString;
}, z.core.$strip>;
export declare const zCurrentUserWritable: z.ZodObject<{
    user: z.ZodObject<{
        id: z.ZodString;
        username: z.ZodString;
        email: z.ZodString;
    }, z.core.$strip>;
    organizations: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        name: z.ZodString;
        role: z.ZodOptional<z.ZodEnum<{
            owner: "owner";
            admin: "admin";
            member: "member";
        }>>;
    }, z.core.$strip>>;
}, z.core.$strip>;
export declare const zTaskIntegrationInputWritable: z.ZodObject<{
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodOptional<z.ZodBoolean>;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    interval: z.ZodOptional<z.ZodString>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zNamedResource: z.ZodObject<{
    name: z.ZodString;
}, z.core.$strip>;
export declare const zOrganizationRole: z.ZodObject<{
    role: z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
    }>;
}, z.core.$strip>;
export declare const zWorkspaceRole: z.ZodObject<{
    role: z.ZodEnum<{
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>;
}, z.core.$strip>;
export declare const zAgentDefinition: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    skillIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
}, z.core.$strip>;
export declare const zMcpServerInput: z.ZodObject<{
    name: z.ZodString;
    url: z.ZodURL;
    headers: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zCodeRepositoryCreate2: z.ZodObject<{
    providerName: z.ZodEnum<{
        github: "github";
    }>;
    url: z.ZodURL;
}, z.core.$strip>;
export declare const zTaskIntegration2: z.ZodObject<{
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodOptional<z.ZodBoolean>;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    interval: z.ZodOptional<z.ZodString>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
/**
 * HTTP process is running
 */
export declare const zGetHealthResponse: z.ZodObject<{
    status: z.ZodEnum<{
        ok: "ok";
    }>;
}, z.core.$strip>;
/**
 * Session cleared
 */
export declare const zLogoutResponse: z.ZodVoid;
/**
 * Internal user profile and organization memberships
 */
export declare const zGetCurrentUserResponse: z.ZodObject<{
    user: z.ZodObject<{
        id: z.ZodString;
        username: z.ZodString;
        email: z.ZodString;
    }, z.core.$strip>;
    organizations: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        name: z.ZodString;
        role: z.ZodOptional<z.ZodEnum<{
            owner: "owner";
            admin: "admin";
            member: "member";
        }>>;
    }, z.core.$strip>>;
    platformAdmin: z.ZodReadonly<z.ZodBoolean>;
}, z.core.$strip>;
/**
 * Organizations the user belongs to
 */
export declare const zListOrganizationsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
    }>>;
}, z.core.$strip>>;
export declare const zCreateOrganizationBody: z.ZodObject<{
    name: z.ZodString;
}, z.core.$strip>;
/**
 * Organization created; creator is Owner
 */
export declare const zCreateOrganizationResponse: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
    }>>;
}, z.core.$strip>;
export declare const zDeleteOrganizationPath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Organization deleted
 */
export declare const zDeleteOrganizationResponse: z.ZodVoid;
export declare const zGetOrganizationPath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Organization
 */
export declare const zGetOrganizationResponse: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
    }>>;
}, z.core.$strip>;
export declare const zUpdateOrganizationBody: z.ZodObject<{
    name: z.ZodString;
}, z.core.$strip>;
export declare const zUpdateOrganizationPath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Updated organization
 */
export declare const zUpdateOrganizationResponse: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
    }>>;
}, z.core.$strip>;
export declare const zListOrganizationMembersPath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Organization members
 */
export declare const zListOrganizationMembersResponse: z.ZodArray<z.ZodObject<{
    user: z.ZodObject<{
        id: z.ZodString;
        username: z.ZodString;
        email: z.ZodString;
    }, z.core.$strip>;
    role: z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>;
}, z.core.$strip>>;
export declare const zRemoveOrganizationMemberPath: z.ZodObject<{
    orgID: z.ZodString;
    userID: z.ZodString;
}, z.core.$strip>;
/**
 * Membership removed
 */
export declare const zRemoveOrganizationMemberResponse: z.ZodVoid;
export declare const zSetOrganizationMemberBody: z.ZodObject<{
    role: z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
    }>;
}, z.core.$strip>;
export declare const zSetOrganizationMemberPath: z.ZodObject<{
    orgID: z.ZodString;
    userID: z.ZodString;
}, z.core.$strip>;
/**
 * Membership updated
 */
export declare const zSetOrganizationMemberResponse: z.ZodObject<{
    status: z.ZodString;
}, z.core.$strip>;
export declare const zListWorkspacesPath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspaces accessible to the user
 */
export declare const zListWorkspacesResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    organizationId: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>>;
}, z.core.$strip>>;
export declare const zCreateWorkspaceBody: z.ZodObject<{
    name: z.ZodString;
}, z.core.$strip>;
export declare const zCreateWorkspacePath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace created
 */
export declare const zCreateWorkspaceResponse: z.ZodObject<{
    id: z.ZodString;
    organizationId: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>>;
}, z.core.$strip>;
export declare const zDeleteWorkspacePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace deleted
 */
export declare const zDeleteWorkspaceResponse: z.ZodVoid;
export declare const zGetWorkspacePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace
 */
export declare const zGetWorkspaceResponse: z.ZodObject<{
    id: z.ZodString;
    organizationId: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>>;
}, z.core.$strip>;
export declare const zUpdateWorkspaceBody: z.ZodObject<{
    name: z.ZodString;
}, z.core.$strip>;
export declare const zUpdateWorkspacePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Updated workspace
 */
export declare const zUpdateWorkspaceResponse: z.ZodObject<{
    id: z.ZodString;
    organizationId: z.ZodString;
    name: z.ZodString;
    role: z.ZodOptional<z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>>;
}, z.core.$strip>;
export declare const zListWorkspaceMembersPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace members
 */
export declare const zListWorkspaceMembersResponse: z.ZodArray<z.ZodObject<{
    user: z.ZodObject<{
        id: z.ZodString;
        username: z.ZodString;
        email: z.ZodString;
    }, z.core.$strip>;
    role: z.ZodEnum<{
        owner: "owner";
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>;
}, z.core.$strip>>;
export declare const zRemoveWorkspaceMemberPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    userID: z.ZodString;
}, z.core.$strip>;
/**
 * Membership removed
 */
export declare const zRemoveWorkspaceMemberResponse: z.ZodVoid;
export declare const zSetWorkspaceMemberBody: z.ZodObject<{
    role: z.ZodEnum<{
        admin: "admin";
        member: "member";
        viewer: "viewer";
    }>;
}, z.core.$strip>;
export declare const zSetWorkspaceMemberPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    userID: z.ZodString;
}, z.core.$strip>;
/**
 * Membership updated
 */
export declare const zSetWorkspaceMemberResponse: z.ZodObject<{
    status: z.ZodString;
}, z.core.$strip>;
export declare const zListCodeRepositoriesPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace code repositories
 */
export declare const zListCodeRepositoriesResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
    }>;
    url: z.ZodURL;
}, z.core.$strip>>;
export declare const zCreateCodeRepositoryBody: z.ZodObject<{
    providerName: z.ZodEnum<{
        github: "github";
    }>;
    url: z.ZodURL;
}, z.core.$strip>;
export declare const zCreateCodeRepositoryPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Code repository registered
 */
export declare const zCreateCodeRepositoryResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
    }>;
    url: z.ZodURL;
}, z.core.$strip>;
export declare const zDeleteCodeRepositoryPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    repositoryID: z.ZodString;
}, z.core.$strip>;
/**
 * Code repository removed
 */
export declare const zDeleteCodeRepositoryResponse: z.ZodVoid;
export declare const zGetCodeRepositoryPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    repositoryID: z.ZodString;
}, z.core.$strip>;
/**
 * Registered code repository
 */
export declare const zGetCodeRepositoryResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
    }>;
    url: z.ZodURL;
}, z.core.$strip>;
export declare const zCloneCodeRepositoryPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    repositoryID: z.ZodString;
}, z.core.$strip>;
/**
 * Repository cloned
 */
export declare const zCloneCodeRepositoryResponse: z.ZodVoid;
export declare const zSyncCodeRepositoryMainPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    repositoryID: z.ZodString;
}, z.core.$strip>;
/**
 * Checkout reset to the latest remote main branch
 */
export declare const zSyncCodeRepositoryMainResponse: z.ZodVoid;
export declare const zListTaskIntegrationsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Configured task integrations
 */
export declare const zListTaskIntegrationsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodBoolean;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodArray<z.ZodString>;
    interval: z.ZodString;
    lastSynced: z.ZodOptional<z.ZodISODateTime>;
    lastError: z.ZodOptional<z.ZodString>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateTaskIntegrationBody: z.ZodObject<{
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodOptional<z.ZodBoolean>;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    interval: z.ZodOptional<z.ZodString>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zCreateTaskIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Integration configured
 */
export declare const zCreateTaskIntegrationResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodBoolean;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodArray<z.ZodString>;
    interval: z.ZodString;
    lastSynced: z.ZodOptional<z.ZodISODateTime>;
    lastError: z.ZodOptional<z.ZodString>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
/**
 * Available providers and their config schemas
 */
export declare const zListTaskIntegrationProvidersResponse: z.ZodArray<z.ZodObject<{
    provider: z.ZodString;
    configSchema: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>>;
export declare const zValidateTaskIntegrationBody: z.ZodObject<{
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodOptional<z.ZodBoolean>;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    interval: z.ZodOptional<z.ZodString>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zValidateTaskIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Connectivity validated
 */
export declare const zValidateTaskIntegrationResponse: z.ZodObject<{
    status: z.ZodString;
}, z.core.$strip>;
export declare const zDeleteTaskIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    integrationID: z.ZodString;
}, z.core.$strip>;
/**
 * Integration deleted
 */
export declare const zDeleteTaskIntegrationResponse: z.ZodVoid;
export declare const zUpdateTaskIntegrationBody: z.ZodObject<{
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodOptional<z.ZodBoolean>;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    interval: z.ZodOptional<z.ZodString>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zUpdateTaskIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    integrationID: z.ZodString;
}, z.core.$strip>;
/**
 * Integration updated
 */
export declare const zUpdateTaskIntegrationResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodBoolean;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodArray<z.ZodString>;
    interval: z.ZodString;
    lastSynced: z.ZodOptional<z.ZodISODateTime>;
    lastError: z.ZodOptional<z.ZodString>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zSyncTaskIntegrationNowPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    integrationID: z.ZodString;
}, z.core.$strip>;
/**
 * Synchronization completed
 */
export declare const zSyncTaskIntegrationNowResponse: z.ZodVoid;
export declare const zListMcpServersPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace MCP servers
 */
export declare const zListMcpServersResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    url: z.ZodURL;
    hasHeaders: z.ZodBoolean;
}, z.core.$strip>>;
export declare const zCreateMcpServerBody: z.ZodObject<{
    name: z.ZodString;
    url: z.ZodURL;
    headers: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zCreateMcpServerPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * MCP server created
 */
export declare const zCreateMcpServerResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    url: z.ZodURL;
    hasHeaders: z.ZodBoolean;
}, z.core.$strip>;
export declare const zDeleteMcpServerPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    serverID: z.ZodString;
}, z.core.$strip>;
/**
 * MCP server deleted and removed from agent selections
 */
export declare const zDeleteMcpServerResponse: z.ZodVoid;
export declare const zUpdateMcpServerBody: z.ZodObject<{
    name: z.ZodString;
    url: z.ZodURL;
    headers: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zUpdateMcpServerPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    serverID: z.ZodString;
}, z.core.$strip>;
/**
 * MCP server updated; omitted headers preserve existing credentials
 */
export declare const zUpdateMcpServerResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    url: z.ZodURL;
    hasHeaders: z.ZodBoolean;
}, z.core.$strip>;
export declare const zListAgentsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace agents
 */
export declare const zListAgentsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
}, z.core.$strip>>;
export declare const zCreateAgentBody: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    skillIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
}, z.core.$strip>;
export declare const zCreateAgentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Agent created
 */
export declare const zCreateAgentResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
}, z.core.$strip>;
export declare const zDeleteAgentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    agentID: z.ZodString;
}, z.core.$strip>;
/**
 * Agent deleted
 */
export declare const zDeleteAgentResponse: z.ZodVoid;
export declare const zGetAgentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    agentID: z.ZodString;
}, z.core.$strip>;
/**
 * Agent definition
 */
export declare const zGetAgentResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
}, z.core.$strip>;
export declare const zUpdateAgentBody: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    skillIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
}, z.core.$strip>;
export declare const zUpdateAgentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    agentID: z.ZodString;
}, z.core.$strip>;
/**
 * Agent updated
 */
export declare const zUpdateAgentResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
}, z.core.$strip>;
export declare const zListTasksPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace tasks
 */
export declare const zListTasksResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    status: z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    components: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        workspaceId: z.ZodString;
        identifier: z.ZodString;
        name: z.ZodString;
        repositoryId: z.ZodString;
        description: z.ZodString;
        createdAt: z.ZodISODateTime;
        updatedAt: z.ZodISODateTime;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateTaskBody: z.ZodObject<{
    title: z.ZodString;
    description: z.ZodOptional<z.ZodString>;
    status: z.ZodOptional<z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    componentIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
}, z.core.$strip>;
export declare const zCreateTaskPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Created task
 */
export declare const zCreateTaskResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    status: z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    components: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        workspaceId: z.ZodString;
        identifier: z.ZodString;
        name: z.ZodString;
        repositoryId: z.ZodString;
        description: z.ZodString;
        createdAt: z.ZodISODateTime;
        updatedAt: z.ZodISODateTime;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zGetTaskPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
}, z.core.$strip>;
/**
 * Task
 */
export declare const zGetTaskResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    status: z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    components: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        workspaceId: z.ZodString;
        identifier: z.ZodString;
        name: z.ZodString;
        repositoryId: z.ZodString;
        description: z.ZodString;
        createdAt: z.ZodISODateTime;
        updatedAt: z.ZodISODateTime;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zUpdateTaskBody: z.ZodObject<{
    title: z.ZodString;
    description: z.ZodOptional<z.ZodString>;
    status: z.ZodOptional<z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    componentIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
}, z.core.$strip>;
export declare const zUpdateTaskPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
}, z.core.$strip>;
/**
 * Updated task
 */
export declare const zUpdateTaskResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    status: z.ZodEnum<{
        done: "done";
        todo: "todo";
        in_progress: "in_progress";
        in_review: "in_review";
    }>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    components: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        workspaceId: z.ZodString;
        identifier: z.ZodString;
        name: z.ZodString;
        repositoryId: z.ZodString;
        description: z.ZodString;
        createdAt: z.ZodISODateTime;
        updatedAt: z.ZodISODateTime;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zListTaskRunsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
}, z.core.$strip>;
/**
 * Task run history
 */
export declare const zListTaskRunsResponse: z.ZodArray<z.ZodObject<{
    runId: z.ZodString;
    taskId: z.ZodString;
    status: z.ZodString;
    failureStage: z.ZodOptional<z.ZodString>;
    failureReason: z.ZodOptional<z.ZodString>;
    lastSequence: z.ZodInt;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zListTaskRunEventsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zListTaskRunEventsQuery: z.ZodObject<{
    afterSequence: z.ZodOptional<z.ZodInt>;
}, z.core.$strip>;
/**
 * Ordered run activity
 */
export declare const zListTaskRunEventsResponse: z.ZodArray<z.ZodObject<{
    runId: z.ZodString;
    sequence: z.ZodInt;
    payload: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>>;
export declare const zSendTaskRunPromptBody: z.ZodObject<{
    commandId: z.ZodString;
    text: z.ZodString;
}, z.core.$strip>;
export declare const zSendTaskRunPromptPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
/**
 * Prompt accepted
 */
export declare const zSendTaskRunPromptResponse: z.ZodObject<{
    commandId: z.ZodString;
}, z.core.$strip>;
export declare const zCancelTaskRunPromptBody: z.ZodObject<{
    commandId: z.ZodString;
}, z.core.$strip>;
export declare const zCancelTaskRunPromptPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zRespondTaskRunPermissionBody: z.ZodObject<{
    requestId: z.ZodString;
    sessionId: z.ZodString;
    optionId: z.ZodString;
}, z.core.$strip>;
export declare const zRespondTaskRunPermissionPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zListTaskCommentsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
}, z.core.$strip>;
/**
 * Task comments
 */
export declare const zListTaskCommentsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    taskId: z.ZodString;
    authorUserId: z.ZodString;
    body: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateTaskCommentBody: z.ZodObject<{
    body: z.ZodString;
}, z.core.$strip>;
export declare const zCreateTaskCommentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
}, z.core.$strip>;
/**
 * Created comment
 */
export declare const zCreateTaskCommentResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    taskId: z.ZodString;
    authorUserId: z.ZodString;
    body: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zListComponentsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace components
 */
export declare const zListComponentsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    identifier: z.ZodString;
    name: z.ZodString;
    repositoryId: z.ZodString;
    description: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateComponentBody: z.ZodObject<{
    identifier: z.ZodString;
    name: z.ZodString;
    repositoryId: z.ZodString;
    description: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zCreateComponentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Created component
 */
export declare const zCreateComponentResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    identifier: z.ZodString;
    name: z.ZodString;
    repositoryId: z.ZodString;
    description: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zUpdateComponentBody: z.ZodObject<{
    identifier: z.ZodString;
    name: z.ZodString;
    repositoryId: z.ZodString;
    description: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zUpdateComponentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    componentID: z.ZodString;
}, z.core.$strip>;
/**
 * Updated component
 */
export declare const zUpdateComponentResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    identifier: z.ZodString;
    name: z.ZodString;
    repositoryId: z.ZodString;
    description: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
/**
 * Global skills
 */
export declare const zListGlobalSkillsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateGlobalSkillBody: z.ZodObject<{
    skillMD: z.ZodString;
}, z.core.$strip>;
/**
 * Created skill
 */
export declare const zCreateGlobalSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zDeleteGlobalSkillPath: z.ZodObject<{
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Deleted skill
 */
export declare const zDeleteGlobalSkillResponse: z.ZodVoid;
export declare const zGetGlobalSkillPath: z.ZodObject<{
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Skill
 */
export declare const zGetGlobalSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zUpdateGlobalSkillBody: z.ZodObject<{
    skillMD: z.ZodString;
}, z.core.$strip>;
export declare const zUpdateGlobalSkillPath: z.ZodObject<{
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Updated skill
 */
export declare const zUpdateGlobalSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zListOrganizationSkillsPath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Organization skills
 */
export declare const zListOrganizationSkillsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateOrganizationSkillBody: z.ZodObject<{
    skillMD: z.ZodString;
}, z.core.$strip>;
export declare const zCreateOrganizationSkillPath: z.ZodObject<{
    orgID: z.ZodString;
}, z.core.$strip>;
/**
 * Created skill
 */
export declare const zCreateOrganizationSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zDeleteOrganizationSkillPath: z.ZodObject<{
    orgID: z.ZodString;
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Deleted skill
 */
export declare const zDeleteOrganizationSkillResponse: z.ZodVoid;
export declare const zGetOrganizationSkillPath: z.ZodObject<{
    orgID: z.ZodString;
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Skill
 */
export declare const zGetOrganizationSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zUpdateOrganizationSkillBody: z.ZodObject<{
    skillMD: z.ZodString;
}, z.core.$strip>;
export declare const zUpdateOrganizationSkillPath: z.ZodObject<{
    orgID: z.ZodString;
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Updated skill
 */
export declare const zUpdateOrganizationSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zListWorkspaceSkillsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Skills visible in workspace
 */
export declare const zListWorkspaceSkillsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateWorkspaceSkillBody: z.ZodObject<{
    skillMD: z.ZodString;
}, z.core.$strip>;
export declare const zCreateWorkspaceSkillPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Created skill
 */
export declare const zCreateWorkspaceSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zDeleteWorkspaceSkillPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Deleted skill
 */
export declare const zDeleteWorkspaceSkillResponse: z.ZodVoid;
export declare const zGetWorkspaceSkillPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Skill
 */
export declare const zGetWorkspaceSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zUpdateWorkspaceSkillBody: z.ZodObject<{
    skillMD: z.ZodString;
}, z.core.$strip>;
export declare const zUpdateWorkspaceSkillPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    skillID: z.ZodString;
}, z.core.$strip>;
/**
 * Updated skill
 */
export declare const zUpdateWorkspaceSkillResponse: z.ZodObject<{
    id: z.ZodString;
    scope: z.ZodEnum<{
        global: "global";
        organization: "organization";
        workspace: "workspace";
    }>;
    organizationId: z.ZodOptional<z.ZodString>;
    workspaceId: z.ZodOptional<z.ZodString>;
    skillMD: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;

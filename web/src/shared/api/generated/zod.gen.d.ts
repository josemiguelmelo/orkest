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
export declare const zWorkspaceEnvironmentInput: z.ZodObject<{
    value: z.ZodString;
    secret: z.ZodDefault<z.ZodOptional<z.ZodBoolean>>;
}, z.core.$strip>;
export declare const zWorkspaceEnvironmentValue: z.ZodObject<{
    name: z.ZodString;
    secret: z.ZodBoolean;
    value: z.ZodOptional<z.ZodString>;
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
export declare const zArtifactContent: z.ZodIntersection<z.ZodUnknown, z.ZodObject<{
    mediaType: z.ZodString;
    text: z.ZodOptional<z.ZodString>;
    url: z.ZodOptional<z.ZodURL>;
    metadata: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
}, z.core.$strip>>;
export declare const zPublishArtifactInput: z.ZodObject<{
    artifactId: z.ZodOptional<z.ZodString>;
    expectedRevision: z.ZodCoercedBigInt<unknown>;
    idempotencyKey: z.ZodString;
    kind: z.ZodString;
    title: z.ZodString;
    outputName: z.ZodOptional<z.ZodString>;
    content: z.ZodIntersection<z.ZodUnknown, z.ZodObject<{
        mediaType: z.ZodString;
        text: z.ZodOptional<z.ZodString>;
        url: z.ZodOptional<z.ZodURL>;
        metadata: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    }, z.core.$strip>>;
}, z.core.$strip>;
export declare const zArtifactRevision: z.ZodObject<{
    id: z.ZodString;
    artifactId: z.ZodString;
    number: z.ZodCoercedBigInt<unknown>;
    kind: z.ZodString;
    title: z.ZodString;
    outputName: z.ZodOptional<z.ZodString>;
    content: z.ZodIntersection<z.ZodUnknown, z.ZodObject<{
        mediaType: z.ZodString;
        text: z.ZodOptional<z.ZodString>;
        url: z.ZodOptional<z.ZodURL>;
        metadata: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    }, z.core.$strip>>;
    createdBy: z.ZodString;
    producingRunId: z.ZodOptional<z.ZodString>;
    stageId: z.ZodString;
    stageVersion: z.ZodCoercedBigInt<unknown>;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zArtifactOutput: z.ZodObject<{
    name: z.ZodString;
    kind: z.ZodString;
}, z.core.$strip>;
export declare const zArtifactInput: z.ZodObject<{
    name: z.ZodString;
    sourceStageId: z.ZodString;
    outputName: z.ZodString;
    required: z.ZodBoolean;
}, z.core.$strip>;
export declare const zWorkflowStage: z.ZodObject<{
    artifactInputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
        name: z.ZodString;
        sourceStageId: z.ZodString;
        outputName: z.ZodString;
        required: z.ZodBoolean;
    }, z.core.$strip>>>;
    artifactOutputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
        name: z.ZodString;
        kind: z.ZodString;
    }, z.core.$strip>>>;
    id: z.ZodString;
    name: z.ZodString;
    description: z.ZodString;
    customPrompt: z.ZodOptional<z.ZodString>;
    responsibleAgentId: z.ZodString;
}, z.core.$strip>;
export declare const zWorkflowAutomationRule: z.ZodObject<{
    id: z.ZodString;
    enabled: z.ZodBoolean;
    event: z.ZodEnum<{
        stage_entered: "stage_entered";
        run_succeeded: "run_succeeded";
        stage_completed: "stage_completed";
    }>;
    sourceStageId: z.ZodString;
    action: z.ZodEnum<{
        move_task: "move_task";
        start_work: "start_work";
    }>;
    targetStageId: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zWorkflowInput: z.ZodObject<{
    name: z.ZodString;
}, z.core.$strip>;
export declare const zWorkspaceWorkflow: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    version: z.ZodCoercedBigInt<unknown>;
    initialStageId: z.ZodString;
    closedStageId: z.ZodString;
    rules: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        enabled: z.ZodBoolean;
        event: z.ZodEnum<{
            stage_entered: "stage_entered";
            run_succeeded: "run_succeeded";
            stage_completed: "stage_completed";
        }>;
        sourceStageId: z.ZodString;
        action: z.ZodEnum<{
            move_task: "move_task";
            start_work: "start_work";
        }>;
        targetStageId: z.ZodOptional<z.ZodString>;
    }, z.core.$strip>>;
    configuredBy: z.ZodOptional<z.ZodString>;
    stages: z.ZodArray<z.ZodObject<{
        artifactInputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            sourceStageId: z.ZodString;
            outputName: z.ZodString;
            required: z.ZodBoolean;
        }, z.core.$strip>>>;
        artifactOutputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            kind: z.ZodString;
        }, z.core.$strip>>>;
        id: z.ZodString;
        name: z.ZodString;
        description: z.ZodString;
        customPrompt: z.ZodOptional<z.ZodString>;
        responsibleAgentId: z.ZodString;
    }, z.core.$strip>>;
}, z.core.$strip>;
export declare const zTaskTransition: z.ZodObject<{
    stageId: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodCoercedBigInt<unknown>;
    action: z.ZodEnum<{
        move: "move";
        start: "start";
        complete: "complete";
    }>;
    commandId: z.ZodString;
    sessionMode: z.ZodOptional<z.ZodEnum<{
        continue: "continue";
        new: "new";
    }>>;
    promptText: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zTaskInput: z.ZodObject<{
    startWork: z.ZodOptional<z.ZodBoolean>;
    sessionMode: z.ZodOptional<z.ZodEnum<{
        continue: "continue";
        new: "new";
    }>>;
    promptText: z.ZodOptional<z.ZodString>;
    commandId: z.ZodOptional<z.ZodString>;
    title: z.ZodString;
    workflowId: z.ZodOptional<z.ZodString>;
    description: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    status: z.ZodOptional<z.ZodString>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    repositoryIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
}, z.core.$strip>;
export declare const zStageWorkState: z.ZodEnum<{
    idle: "idle";
    queued: "queued";
    running: "running";
    completed: "completed";
    failed: "failed";
}>;
export declare const zTaskRepository: z.ZodObject<{
    id: z.ZodString;
    providerName: z.ZodString;
    name: z.ZodString;
    url: z.ZodURL;
}, z.core.$strip>;
export declare const zTask: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    workflowId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    automationPending: z.ZodOptional<z.ZodBoolean>;
    stageWorkState: z.ZodOptional<z.ZodEnum<{
        idle: "idle";
        queued: "queued";
        running: "running";
        completed: "completed";
        failed: "failed";
    }>>;
    closed: z.ZodOptional<z.ZodBoolean>;
    status: z.ZodString;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        providerName: z.ZodString;
        name: z.ZodString;
        url: z.ZodURL;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
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
    executionImage: z.ZodOptional<z.ZodString>;
    recoveryBlocked: z.ZodOptional<z.ZodBoolean>;
    executionState: z.ZodOptional<z.ZodString>;
    queuePaused: z.ZodOptional<z.ZodBoolean>;
    queueDepth: z.ZodOptional<z.ZodInt>;
    runId: z.ZodString;
    taskSessionId: z.ZodOptional<z.ZodString>;
    taskId: z.ZodString;
    status: z.ZodString;
    failureStage: z.ZodOptional<z.ZodString>;
    failureReason: z.ZodOptional<z.ZodString>;
    lastSequence: z.ZodInt;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zDirectAgentConversation: z.ZodObject<{
    id: z.ZodString;
    agentId: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zQuestionResponseRequest: z.ZodObject<{
    taskSessionId: z.ZodString;
    sessionId: z.ZodString;
    commandId: z.ZodString;
    submissionId: z.ZodString;
    answer: z.ZodObject<{
        action: z.ZodEnum<{
            accept: "accept";
            decline: "decline";
            cancel: "cancel";
        }>;
        content: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    }, z.core.$strip>;
}, z.core.$strip>;
export declare const zTaskRunEvent: z.ZodObject<{
    runId: z.ZodString;
    sequence: z.ZodInt;
    payload: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>;
export declare const zExecutionProfileInput: z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    generation: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
}, z.core.$strip>;
export declare const zExecutionProfile: z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    generation: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    generation: z.ZodCoercedBigInt<unknown>;
    status: z.ZodString;
    executionImage: z.ZodString;
    buildError: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zExecutionProfileBuild: z.ZodObject<{
    id: z.ZodString;
    profileId: z.ZodString;
    workspaceId: z.ZodString;
    generation: z.ZodCoercedBigInt<unknown>;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    baseImage: z.ZodString;
    runtimeVersion: z.ZodOptional<z.ZodString>;
    resolvedToolchain: z.ZodOptional<z.ZodString>;
    resolvedBase: z.ZodOptional<z.ZodString>;
    destination: z.ZodString;
    status: z.ZodString;
    executionImage: z.ZodString;
    buildError: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zAgent: z.ZodObject<{
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
    hasConfiguration: z.ZodBoolean;
}, z.core.$strip>;
export declare const zAgentRuntimeProfileInput: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
}, z.core.$strip>;
export declare const zAgentRuntimeProfile: z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
    hasConfiguration: z.ZodOptional<z.ZodBoolean>;
}, z.core.$strip>>;
export declare const zRuntimeConfigurationOption: z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    required: z.ZodBoolean;
    sensitive: z.ZodBoolean;
}, z.core.$strip>;
export declare const zRuntimeProvider: z.ZodObject<{
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    configuration: z.ZodArray<z.ZodObject<{
        name: z.ZodString;
        description: z.ZodString;
        required: z.ZodBoolean;
        sensitive: z.ZodBoolean;
    }, z.core.$strip>>;
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
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    url: z.ZodURL;
    integrationId: z.ZodOptional<z.ZodString>;
    providerRepositoryId: z.ZodOptional<z.ZodString>;
    defaultBranch: z.ZodString;
    name: z.ZodOptional<z.ZodString>;
    private: z.ZodBoolean;
}, z.core.$strip>;
export declare const zCodingIntegrationInput: z.ZodObject<{
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
}, z.core.$strip>;
export declare const zCodingIntegration: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    baseUrl: z.ZodReadonly<z.ZodURL>;
    updatedAt: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zCodingProvider: z.ZodObject<{
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    configSchema: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>;
export declare const zCodingRepository: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    cloneUrl: z.ZodURL;
    private: z.ZodBoolean;
    defaultBranch: z.ZodString;
}, z.core.$strip>;
export declare const zCodingRepositoryPage: z.ZodObject<{
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        name: z.ZodString;
        cloneUrl: z.ZodURL;
        private: z.ZodBoolean;
        defaultBranch: z.ZodString;
    }, z.core.$strip>>;
    nextPage: z.ZodOptional<z.ZodInt>;
}, z.core.$strip>;
export declare const zCodingIntegrationValidation: z.ZodObject<{
    connected: z.ZodBoolean;
}, z.core.$strip>;
export declare const zCodeRepositoryCreate: z.ZodObject<{
    integrationId: z.ZodString;
    providerRepositoryId: z.ZodString;
}, z.core.$strip>;
/**
 * Provider-defined config. Retrieve allowed fields and constraints from GET /api/task-integration-providers.
 */
export declare const zTaskIntegrationConfig: z.ZodRecord<z.ZodString, z.ZodUnknown>;
export declare const zTaskIntegrationInput: z.ZodObject<{
    workflowId: z.ZodString;
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
    workflowId: z.ZodString;
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
export declare const zAgentRuntimeProfileInputWritable: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
    configuration: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zAgentRuntimeProfileWritable: z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
    configuration: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
    hasConfiguration: z.ZodOptional<z.ZodBoolean>;
}, z.core.$strip>>;
export declare const zCodingIntegrationInputWritable: z.ZodObject<{
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zCodingIntegrationWritable: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    updatedAt: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zTaskIntegrationInputWritable: z.ZodObject<{
    workflowId: z.ZodString;
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
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    mcpServerIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    skillIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    configuration: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zMcpServerInput: z.ZodObject<{
    name: z.ZodString;
    url: z.ZodURL;
    headers: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zCodeRepositoryCreate2: z.ZodObject<{
    integrationId: z.ZodString;
    providerRepositoryId: z.ZodString;
}, z.core.$strip>;
export declare const zTaskIntegration2: z.ZodObject<{
    workflowId: z.ZodString;
    provider: z.ZodString;
    name: z.ZodString;
    enabled: z.ZodOptional<z.ZodBoolean>;
    config: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    interval: z.ZodOptional<z.ZodString>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
/**
 * Runtime providers and supported environment configuration
 */
export declare const zListRuntimeProvidersResponse: z.ZodArray<z.ZodObject<{
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    configuration: z.ZodArray<z.ZodObject<{
        name: z.ZodString;
        description: z.ZodString;
        required: z.ZodBoolean;
        sensitive: z.ZodBoolean;
    }, z.core.$strip>>;
}, z.core.$strip>>;
export declare const zListAgentRuntimeProfilesPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Runtime profiles
 */
export declare const zListAgentRuntimeProfilesResponse: z.ZodArray<z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
    hasConfiguration: z.ZodOptional<z.ZodBoolean>;
}, z.core.$strip>>>;
export declare const zCreateAgentRuntimeProfileBody: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
    configuration: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zCreateAgentRuntimeProfilePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Runtime profile created and applied to linked agents
 */
export declare const zCreateAgentRuntimeProfileResponse: z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
    hasConfiguration: z.ZodOptional<z.ZodBoolean>;
}, z.core.$strip>>;
export declare const zDeleteAgentRuntimeProfilePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    profileID: z.ZodString;
}, z.core.$strip>;
/**
 * Runtime profile deleted; linked agents retain their last model settings
 */
export declare const zDeleteAgentRuntimeProfileResponse: z.ZodVoid;
export declare const zUpdateAgentRuntimeProfileBody: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
    configuration: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zUpdateAgentRuntimeProfilePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    profileID: z.ZodString;
}, z.core.$strip>;
/**
 * Runtime profile updated and applied to linked agents
 */
export declare const zUpdateAgentRuntimeProfileResponse: z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    modelProvider: z.ZodString;
    modelIdentifier: z.ZodString;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
    hasConfiguration: z.ZodOptional<z.ZodBoolean>;
}, z.core.$strip>>;
export declare const zListExecutionProfilesPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Execution profile operation
 */
export declare const zListExecutionProfilesResponse: z.ZodArray<z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    generation: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    generation: z.ZodCoercedBigInt<unknown>;
    status: z.ZodString;
    executionImage: z.ZodString;
    buildError: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>>;
export declare const zCreateExecutionProfileBody: z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    generation: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
}, z.core.$strip>;
export declare const zCreateExecutionProfilePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Execution profile operation
 */
export declare const zCreateExecutionProfileResponse: z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    generation: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    generation: z.ZodCoercedBigInt<unknown>;
    status: z.ZodString;
    executionImage: z.ZodString;
    buildError: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zUpdateExecutionProfileBody: z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    generation: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
}, z.core.$strip>;
export declare const zUpdateExecutionProfilePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    profileID: z.ZodString;
}, z.core.$strip>;
/**
 * Execution profile operation
 */
export declare const zUpdateExecutionProfileResponse: z.ZodIntersection<z.ZodObject<{
    name: z.ZodString;
    description: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    generation: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
}, z.core.$strip>, z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    generation: z.ZodCoercedBigInt<unknown>;
    status: z.ZodString;
    executionImage: z.ZodString;
    buildError: z.ZodString;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zListExecutionProfileBuildsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    profileID: z.ZodString;
}, z.core.$strip>;
/**
 * Execution profile operation
 */
export declare const zListExecutionProfileBuildsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    profileId: z.ZodString;
    workspaceId: z.ZodString;
    generation: z.ZodCoercedBigInt<unknown>;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    toolchainImage: z.ZodString;
    baseImage: z.ZodString;
    runtimeVersion: z.ZodOptional<z.ZodString>;
    resolvedToolchain: z.ZodOptional<z.ZodString>;
    resolvedBase: z.ZodOptional<z.ZodString>;
    destination: z.ZodString;
    status: z.ZodString;
    executionImage: z.ZodString;
    buildError: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>>;
/**
 * Coding provider catalog
 */
export declare const zListCodingProvidersResponse: z.ZodArray<z.ZodObject<{
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    configSchema: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>>;
export declare const zListCodingIntegrationsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace coding integrations
 */
export declare const zListCodingIntegrationsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    baseUrl: z.ZodReadonly<z.ZodURL>;
    updatedAt: z.ZodOptional<z.ZodString>;
}, z.core.$strip>>;
export declare const zCreateCodingIntegrationBody: z.ZodObject<{
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zCreateCodingIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Coding integration created
 */
export declare const zCreateCodingIntegrationResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodRecord<z.ZodString, z.ZodUnknown>;
    baseUrl: z.ZodReadonly<z.ZodURL>;
    updatedAt: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zDeleteCodingIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    integrationID: z.ZodString;
}, z.core.$strip>;
/**
 * Coding integration deleted
 */
export declare const zDeleteCodingIntegrationResponse: z.ZodVoid;
export declare const zUpdateCodingIntegrationBody: z.ZodObject<{
    name: z.ZodString;
    providerName: z.ZodEnum<{
        github: "github";
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    providerConfig: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    credential: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zUpdateCodingIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    integrationID: z.ZodString;
}, z.core.$strip>;
/**
 * Coding integration updated
 */
export declare const zUpdateCodingIntegrationResponse: z.ZodVoid;
export declare const zValidateCodingIntegrationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    integrationID: z.ZodString;
}, z.core.$strip>;
/**
 * Saved connection validation succeeded
 */
export declare const zValidateCodingIntegrationResponse: z.ZodObject<{
    connected: z.ZodBoolean;
}, z.core.$strip>;
export declare const zBrowseCodingRepositoriesPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    integrationID: z.ZodString;
}, z.core.$strip>;
export declare const zBrowseCodingRepositoriesQuery: z.ZodObject<{
    page: z.ZodDefault<z.ZodOptional<z.ZodInt>>;
    perPage: z.ZodDefault<z.ZodOptional<z.ZodInt>>;
}, z.core.$strip>;
/**
 * Provider repository page
 */
export declare const zBrowseCodingRepositoriesResponse: z.ZodObject<{
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        name: z.ZodString;
        cloneUrl: z.ZodURL;
        private: z.ZodBoolean;
        defaultBranch: z.ZodString;
    }, z.core.$strip>>;
    nextPage: z.ZodOptional<z.ZodInt>;
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
export declare const zListWorkspaceEnvironmentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Variables are returned with values; secrets return only their names and secret status
 */
export declare const zListWorkspaceEnvironmentResponse: z.ZodArray<z.ZodObject<{
    name: z.ZodString;
    secret: z.ZodBoolean;
    value: z.ZodOptional<z.ZodString>;
}, z.core.$strip>>;
export declare const zDeleteWorkspaceEnvironmentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    name: z.ZodString;
}, z.core.$strip>;
/**
 * Value deleted
 */
export declare const zDeleteWorkspaceEnvironmentResponse: z.ZodVoid;
export declare const zPutWorkspaceEnvironmentBody: z.ZodObject<{
    value: z.ZodString;
    secret: z.ZodDefault<z.ZodOptional<z.ZodBoolean>>;
}, z.core.$strip>;
export declare const zPutWorkspaceEnvironmentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    name: z.ZodString;
}, z.core.$strip>;
/**
 * Value updated
 */
export declare const zPutWorkspaceEnvironmentResponse: z.ZodObject<{
    status: z.ZodString;
}, z.core.$strip>;
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
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    url: z.ZodURL;
    integrationId: z.ZodOptional<z.ZodString>;
    providerRepositoryId: z.ZodOptional<z.ZodString>;
    defaultBranch: z.ZodString;
    name: z.ZodOptional<z.ZodString>;
    private: z.ZodBoolean;
}, z.core.$strip>>;
export declare const zCreateCodeRepositoryBody: z.ZodObject<{
    integrationId: z.ZodString;
    providerRepositoryId: z.ZodString;
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
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    url: z.ZodURL;
    integrationId: z.ZodOptional<z.ZodString>;
    providerRepositoryId: z.ZodOptional<z.ZodString>;
    defaultBranch: z.ZodString;
    name: z.ZodOptional<z.ZodString>;
    private: z.ZodBoolean;
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
        gitlab_cloud: "gitlab_cloud";
        gitlab_server: "gitlab_server";
    }>;
    url: z.ZodURL;
    integrationId: z.ZodOptional<z.ZodString>;
    providerRepositoryId: z.ZodOptional<z.ZodString>;
    defaultBranch: z.ZodString;
    name: z.ZodOptional<z.ZodString>;
    private: z.ZodBoolean;
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
export declare const zSyncCodeRepositoryPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    repositoryID: z.ZodString;
}, z.core.$strip>;
/**
 * Shared checkout synced to the registered repository default branch
 */
export declare const zSyncCodeRepositoryResponse: z.ZodVoid;
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
    workflowId: z.ZodString;
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
    workflowId: z.ZodString;
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
    workflowId: z.ZodString;
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
    workflowId: z.ZodString;
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
    workflowId: z.ZodString;
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
    workflowId: z.ZodString;
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
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
    hasConfiguration: z.ZodBoolean;
}, z.core.$strip>>;
export declare const zCreateAgentBody: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    mcpServerIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    skillIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    configuration: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
}, z.core.$strip>;
export declare const zCreateAgentPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Agent created
 */
export declare const zCreateAgentResponse: z.ZodObject<{
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
    hasConfiguration: z.ZodBoolean;
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
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
    hasConfiguration: z.ZodBoolean;
}, z.core.$strip>;
export declare const zUpdateAgentBody: z.ZodObject<{
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    mcpServerIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    skillIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
    configuration: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodString>>;
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
    executionProfileId: z.ZodOptional<z.ZodString>;
    runtimeProfileId: z.ZodOptional<z.ZodString>;
    id: z.ZodString;
    workspaceId: z.ZodString;
    name: z.ZodString;
    runtime: z.ZodEnum<{
        opencode: "opencode";
        "claude-code": "claude-code";
        codex: "codex";
        "github-copilot": "github-copilot";
        kiro: "kiro";
    }>;
    provider: z.ZodString;
    model: z.ZodString;
    description: z.ZodString;
    systemPrompt: z.ZodString;
    permissionPolicy: z.ZodEnum<{
        ask: "ask";
        autonomous: "autonomous";
    }>;
    mcpServerIds: z.ZodArray<z.ZodString>;
    skillIds: z.ZodArray<z.ZodString>;
    hasConfiguration: z.ZodBoolean;
}, z.core.$strip>;
export declare const zListWorkspaceWorkflowsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Workspace workflows
 */
export declare const zListWorkspaceWorkflowsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    version: z.ZodCoercedBigInt<unknown>;
    initialStageId: z.ZodString;
    closedStageId: z.ZodString;
    rules: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        enabled: z.ZodBoolean;
        event: z.ZodEnum<{
            stage_entered: "stage_entered";
            run_succeeded: "run_succeeded";
            stage_completed: "stage_completed";
        }>;
        sourceStageId: z.ZodString;
        action: z.ZodEnum<{
            move_task: "move_task";
            start_work: "start_work";
        }>;
        targetStageId: z.ZodOptional<z.ZodString>;
    }, z.core.$strip>>;
    configuredBy: z.ZodOptional<z.ZodString>;
    stages: z.ZodArray<z.ZodObject<{
        artifactInputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            sourceStageId: z.ZodString;
            outputName: z.ZodString;
            required: z.ZodBoolean;
        }, z.core.$strip>>>;
        artifactOutputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            kind: z.ZodString;
        }, z.core.$strip>>>;
        id: z.ZodString;
        name: z.ZodString;
        description: z.ZodString;
        customPrompt: z.ZodOptional<z.ZodString>;
        responsibleAgentId: z.ZodString;
    }, z.core.$strip>>;
}, z.core.$strip>>;
export declare const zCreateWorkspaceWorkflowBody: z.ZodObject<{
    name: z.ZodString;
}, z.core.$strip>;
export declare const zCreateWorkspaceWorkflowPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Created workflow
 */
export declare const zCreateWorkspaceWorkflowResponse: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    version: z.ZodCoercedBigInt<unknown>;
    initialStageId: z.ZodString;
    closedStageId: z.ZodString;
    rules: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        enabled: z.ZodBoolean;
        event: z.ZodEnum<{
            stage_entered: "stage_entered";
            run_succeeded: "run_succeeded";
            stage_completed: "stage_completed";
        }>;
        sourceStageId: z.ZodString;
        action: z.ZodEnum<{
            move_task: "move_task";
            start_work: "start_work";
        }>;
        targetStageId: z.ZodOptional<z.ZodString>;
    }, z.core.$strip>>;
    configuredBy: z.ZodOptional<z.ZodString>;
    stages: z.ZodArray<z.ZodObject<{
        artifactInputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            sourceStageId: z.ZodString;
            outputName: z.ZodString;
            required: z.ZodBoolean;
        }, z.core.$strip>>>;
        artifactOutputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            kind: z.ZodString;
        }, z.core.$strip>>>;
        id: z.ZodString;
        name: z.ZodString;
        description: z.ZodString;
        customPrompt: z.ZodOptional<z.ZodString>;
        responsibleAgentId: z.ZodString;
    }, z.core.$strip>>;
}, z.core.$strip>;
export declare const zDeleteWorkspaceWorkflowPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    workflowID: z.ZodString;
}, z.core.$strip>;
/**
 * Workflow deleted
 */
export declare const zDeleteWorkspaceWorkflowResponse: z.ZodVoid;
export declare const zGetWorkspaceWorkflowPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    workflowID: z.ZodString;
}, z.core.$strip>;
/**
 * Ordered workspace workflow
 */
export declare const zGetWorkspaceWorkflowResponse: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    version: z.ZodCoercedBigInt<unknown>;
    initialStageId: z.ZodString;
    closedStageId: z.ZodString;
    rules: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        enabled: z.ZodBoolean;
        event: z.ZodEnum<{
            stage_entered: "stage_entered";
            run_succeeded: "run_succeeded";
            stage_completed: "stage_completed";
        }>;
        sourceStageId: z.ZodString;
        action: z.ZodEnum<{
            move_task: "move_task";
            start_work: "start_work";
        }>;
        targetStageId: z.ZodOptional<z.ZodString>;
    }, z.core.$strip>>;
    configuredBy: z.ZodOptional<z.ZodString>;
    stages: z.ZodArray<z.ZodObject<{
        artifactInputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            sourceStageId: z.ZodString;
            outputName: z.ZodString;
            required: z.ZodBoolean;
        }, z.core.$strip>>>;
        artifactOutputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            kind: z.ZodString;
        }, z.core.$strip>>>;
        id: z.ZodString;
        name: z.ZodString;
        description: z.ZodString;
        customPrompt: z.ZodOptional<z.ZodString>;
        responsibleAgentId: z.ZodString;
    }, z.core.$strip>>;
}, z.core.$strip>;
export declare const zSaveWorkspaceWorkflowBody: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    version: z.ZodCoercedBigInt<unknown>;
    initialStageId: z.ZodString;
    closedStageId: z.ZodString;
    rules: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        enabled: z.ZodBoolean;
        event: z.ZodEnum<{
            stage_entered: "stage_entered";
            run_succeeded: "run_succeeded";
            stage_completed: "stage_completed";
        }>;
        sourceStageId: z.ZodString;
        action: z.ZodEnum<{
            move_task: "move_task";
            start_work: "start_work";
        }>;
        targetStageId: z.ZodOptional<z.ZodString>;
    }, z.core.$strip>>;
    configuredBy: z.ZodOptional<z.ZodString>;
    stages: z.ZodArray<z.ZodObject<{
        artifactInputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            sourceStageId: z.ZodString;
            outputName: z.ZodString;
            required: z.ZodBoolean;
        }, z.core.$strip>>>;
        artifactOutputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            kind: z.ZodString;
        }, z.core.$strip>>>;
        id: z.ZodString;
        name: z.ZodString;
        description: z.ZodString;
        customPrompt: z.ZodOptional<z.ZodString>;
        responsibleAgentId: z.ZodString;
    }, z.core.$strip>>;
}, z.core.$strip>;
export declare const zSaveWorkspaceWorkflowPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    workflowID: z.ZodString;
}, z.core.$strip>;
/**
 * Saved workflow with incremented version
 */
export declare const zSaveWorkspaceWorkflowResponse: z.ZodObject<{
    id: z.ZodString;
    name: z.ZodString;
    version: z.ZodCoercedBigInt<unknown>;
    initialStageId: z.ZodString;
    closedStageId: z.ZodString;
    rules: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        enabled: z.ZodBoolean;
        event: z.ZodEnum<{
            stage_entered: "stage_entered";
            run_succeeded: "run_succeeded";
            stage_completed: "stage_completed";
        }>;
        sourceStageId: z.ZodString;
        action: z.ZodEnum<{
            move_task: "move_task";
            start_work: "start_work";
        }>;
        targetStageId: z.ZodOptional<z.ZodString>;
    }, z.core.$strip>>;
    configuredBy: z.ZodOptional<z.ZodString>;
    stages: z.ZodArray<z.ZodObject<{
        artifactInputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            sourceStageId: z.ZodString;
            outputName: z.ZodString;
            required: z.ZodBoolean;
        }, z.core.$strip>>>;
        artifactOutputs: z.ZodOptional<z.ZodArray<z.ZodObject<{
            name: z.ZodString;
            kind: z.ZodString;
        }, z.core.$strip>>>;
        id: z.ZodString;
        name: z.ZodString;
        description: z.ZodString;
        customPrompt: z.ZodOptional<z.ZodString>;
        responsibleAgentId: z.ZodString;
    }, z.core.$strip>>;
}, z.core.$strip>;
export declare const zTransitionTaskBody: z.ZodObject<{
    stageId: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodCoercedBigInt<unknown>;
    action: z.ZodEnum<{
        move: "move";
        start: "start";
        complete: "complete";
    }>;
    commandId: z.ZodString;
    sessionMode: z.ZodOptional<z.ZodEnum<{
        continue: "continue";
        new: "new";
    }>>;
    promptText: z.ZodOptional<z.ZodString>;
}, z.core.$strip>;
export declare const zTransitionTaskPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
}, z.core.$strip>;
/**
 * Task after the transition; any execution start is durably queued
 */
export declare const zTransitionTaskResponse: z.ZodObject<{
    id: z.ZodString;
    workspaceId: z.ZodString;
    workflowId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    automationPending: z.ZodOptional<z.ZodBoolean>;
    stageWorkState: z.ZodOptional<z.ZodEnum<{
        idle: "idle";
        queued: "queued";
        running: "running";
        completed: "completed";
        failed: "failed";
    }>>;
    closed: z.ZodOptional<z.ZodBoolean>;
    status: z.ZodString;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        providerName: z.ZodString;
        name: z.ZodString;
        url: z.ZodURL;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
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
    workflowId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    automationPending: z.ZodOptional<z.ZodBoolean>;
    stageWorkState: z.ZodOptional<z.ZodEnum<{
        idle: "idle";
        queued: "queued";
        running: "running";
        completed: "completed";
        failed: "failed";
    }>>;
    closed: z.ZodOptional<z.ZodBoolean>;
    status: z.ZodString;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        providerName: z.ZodString;
        name: z.ZodString;
        url: z.ZodURL;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateTaskBody: z.ZodObject<{
    startWork: z.ZodOptional<z.ZodBoolean>;
    sessionMode: z.ZodOptional<z.ZodEnum<{
        continue: "continue";
        new: "new";
    }>>;
    promptText: z.ZodOptional<z.ZodString>;
    commandId: z.ZodOptional<z.ZodString>;
    title: z.ZodString;
    workflowId: z.ZodOptional<z.ZodString>;
    description: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    status: z.ZodOptional<z.ZodString>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    repositoryIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
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
    workflowId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    automationPending: z.ZodOptional<z.ZodBoolean>;
    stageWorkState: z.ZodOptional<z.ZodEnum<{
        idle: "idle";
        queued: "queued";
        running: "running";
        completed: "completed";
        failed: "failed";
    }>>;
    closed: z.ZodOptional<z.ZodBoolean>;
    status: z.ZodString;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        providerName: z.ZodString;
        name: z.ZodString;
        url: z.ZodURL;
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
    workflowId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    automationPending: z.ZodOptional<z.ZodBoolean>;
    stageWorkState: z.ZodOptional<z.ZodEnum<{
        idle: "idle";
        queued: "queued";
        running: "running";
        completed: "completed";
        failed: "failed";
    }>>;
    closed: z.ZodOptional<z.ZodBoolean>;
    status: z.ZodString;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        providerName: z.ZodString;
        name: z.ZodString;
        url: z.ZodURL;
    }, z.core.$strip>>;
    createdAt: z.ZodISODateTime;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zUpdateTaskBody: z.ZodObject<{
    startWork: z.ZodOptional<z.ZodBoolean>;
    sessionMode: z.ZodOptional<z.ZodEnum<{
        continue: "continue";
        new: "new";
    }>>;
    promptText: z.ZodOptional<z.ZodString>;
    commandId: z.ZodOptional<z.ZodString>;
    title: z.ZodString;
    workflowId: z.ZodOptional<z.ZodString>;
    description: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    status: z.ZodOptional<z.ZodString>;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodOptional<z.ZodArray<z.ZodString>>;
    repositoryIds: z.ZodOptional<z.ZodArray<z.ZodString>>;
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
    workflowId: z.ZodString;
    title: z.ZodString;
    description: z.ZodString;
    sourceProvider: z.ZodString;
    sourceId: z.ZodOptional<z.ZodString>;
    sourceUrl: z.ZodOptional<z.ZodString>;
    stageVersion: z.ZodOptional<z.ZodCoercedBigInt<unknown>>;
    automationPending: z.ZodOptional<z.ZodBoolean>;
    stageWorkState: z.ZodOptional<z.ZodEnum<{
        idle: "idle";
        queued: "queued";
        running: "running";
        completed: "completed";
        failed: "failed";
    }>>;
    closed: z.ZodOptional<z.ZodBoolean>;
    status: z.ZodString;
    assigneeAgentId: z.ZodOptional<z.ZodString>;
    labels: z.ZodArray<z.ZodString>;
    repositories: z.ZodArray<z.ZodObject<{
        id: z.ZodString;
        providerName: z.ZodString;
        name: z.ZodString;
        url: z.ZodURL;
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
    executionImage: z.ZodOptional<z.ZodString>;
    recoveryBlocked: z.ZodOptional<z.ZodBoolean>;
    executionState: z.ZodOptional<z.ZodString>;
    queuePaused: z.ZodOptional<z.ZodBoolean>;
    queueDepth: z.ZodOptional<z.ZodInt>;
    runId: z.ZodString;
    taskSessionId: z.ZodOptional<z.ZodString>;
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
export declare const zListDirectAgentConversationsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Direct conversations
 */
export declare const zListDirectAgentConversationsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    agentId: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zCreateDirectAgentConversationBody: z.ZodObject<{
    agentId: z.ZodString;
}, z.core.$strip>;
export declare const zCreateDirectAgentConversationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
}, z.core.$strip>;
/**
 * Conversation created and agent session started
 */
export declare const zCreateDirectAgentConversationResponse: z.ZodObject<{
    id: z.ZodString;
    agentId: z.ZodString;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>;
export declare const zListDirectAgentConversationRunsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
}, z.core.$strip>;
/**
 * Conversation run history
 */
export declare const zListDirectAgentConversationRunsResponse: z.ZodArray<z.ZodObject<{
    executionImage: z.ZodOptional<z.ZodString>;
    recoveryBlocked: z.ZodOptional<z.ZodBoolean>;
    executionState: z.ZodOptional<z.ZodString>;
    queuePaused: z.ZodOptional<z.ZodBoolean>;
    queueDepth: z.ZodOptional<z.ZodInt>;
    runId: z.ZodString;
    taskSessionId: z.ZodOptional<z.ZodString>;
    taskId: z.ZodString;
    status: z.ZodString;
    failureStage: z.ZodOptional<z.ZodString>;
    failureReason: z.ZodOptional<z.ZodString>;
    lastSequence: z.ZodInt;
    updatedAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zDeleteDirectAgentConversationPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
}, z.core.$strip>;
/**
 * Conversation and saved run history deleted
 */
export declare const zDeleteDirectAgentConversationResponse: z.ZodVoid;
export declare const zListDirectAgentConversationEventsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zListDirectAgentConversationEventsQuery: z.ZodObject<{
    afterSequence: z.ZodOptional<z.ZodInt>;
}, z.core.$strip>;
/**
 * Ordered run activity
 */
export declare const zListDirectAgentConversationEventsResponse: z.ZodArray<z.ZodObject<{
    runId: z.ZodString;
    sequence: z.ZodInt;
    payload: z.ZodRecord<z.ZodString, z.ZodUnknown>;
}, z.core.$strip>>;
export declare const zSendDirectAgentConversationPromptBody: z.ZodObject<{
    commandId: z.ZodString;
    text: z.ZodString;
}, z.core.$strip>;
export declare const zSendDirectAgentConversationPromptPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zCancelDirectAgentConversationPromptBody: z.ZodObject<{
    commandId: z.ZodString;
}, z.core.$strip>;
export declare const zCancelDirectAgentConversationPromptPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zResumeDirectAgentConversationQueuePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zDiscardDirectAgentConversationQueuePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zRespondDirectAgentConversationPermissionBody: z.ZodObject<{
    requestId: z.ZodString;
    sessionId: z.ZodString;
    optionId: z.ZodString;
}, z.core.$strip>;
export declare const zRespondDirectAgentConversationPermissionPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zListTaskArtifactsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
}, z.core.$strip>;
/**
 * Artifact revisions
 */
export declare const zListTaskArtifactsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    artifactId: z.ZodString;
    number: z.ZodCoercedBigInt<unknown>;
    kind: z.ZodString;
    title: z.ZodString;
    outputName: z.ZodOptional<z.ZodString>;
    content: z.ZodIntersection<z.ZodUnknown, z.ZodObject<{
        mediaType: z.ZodString;
        text: z.ZodOptional<z.ZodString>;
        url: z.ZodOptional<z.ZodURL>;
        metadata: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    }, z.core.$strip>>;
    createdBy: z.ZodString;
    producingRunId: z.ZodOptional<z.ZodString>;
    stageId: z.ZodString;
    stageVersion: z.ZodCoercedBigInt<unknown>;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>>;
export declare const zListArtifactRevisionsPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    artifactID: z.ZodString;
}, z.core.$strip>;
/**
 * Artifact revisions
 */
export declare const zListArtifactRevisionsResponse: z.ZodArray<z.ZodObject<{
    id: z.ZodString;
    artifactId: z.ZodString;
    number: z.ZodCoercedBigInt<unknown>;
    kind: z.ZodString;
    title: z.ZodString;
    outputName: z.ZodOptional<z.ZodString>;
    content: z.ZodIntersection<z.ZodUnknown, z.ZodObject<{
        mediaType: z.ZodString;
        text: z.ZodOptional<z.ZodString>;
        url: z.ZodOptional<z.ZodURL>;
        metadata: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    }, z.core.$strip>>;
    createdBy: z.ZodString;
    producingRunId: z.ZodOptional<z.ZodString>;
    stageId: z.ZodString;
    stageVersion: z.ZodCoercedBigInt<unknown>;
    createdAt: z.ZodISODateTime;
}, z.core.$strip>>;
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
export declare const zResumeTaskRunQueuePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zDiscardTaskRunQueuePath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    runID: z.ZodString;
}, z.core.$strip>;
export declare const zRespondTaskRunQuestionBody: z.ZodObject<{
    taskSessionId: z.ZodString;
    sessionId: z.ZodString;
    commandId: z.ZodString;
    submissionId: z.ZodString;
    answer: z.ZodObject<{
        action: z.ZodEnum<{
            accept: "accept";
            decline: "decline";
            cancel: "cancel";
        }>;
        content: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    }, z.core.$strip>;
}, z.core.$strip>;
export declare const zRespondTaskRunQuestionPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    taskID: z.ZodString;
    runID: z.ZodString;
    questionID: z.ZodString;
}, z.core.$strip>;
export declare const zRespondDirectAgentConversationQuestionBody: z.ZodObject<{
    taskSessionId: z.ZodString;
    sessionId: z.ZodString;
    commandId: z.ZodString;
    submissionId: z.ZodString;
    answer: z.ZodObject<{
        action: z.ZodEnum<{
            accept: "accept";
            decline: "decline";
            cancel: "cancel";
        }>;
        content: z.ZodOptional<z.ZodRecord<z.ZodString, z.ZodUnknown>>;
    }, z.core.$strip>;
}, z.core.$strip>;
export declare const zRespondDirectAgentConversationQuestionPath: z.ZodObject<{
    orgID: z.ZodString;
    workspaceID: z.ZodString;
    conversationID: z.ZodString;
    runID: z.ZodString;
    questionID: z.ZodString;
}, z.core.$strip>;

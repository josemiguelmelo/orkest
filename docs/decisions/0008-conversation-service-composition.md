# RunController composes ConversationService

Status: Accepted. Supersedes the shared controller/conversation locking described
in decision 0006. Transport contracts and recovery limitations remain unchanged.

RunController owns run configuration, the single launch attempt, and provider
process shutdown. It composes a named *ConversationService and delegates Prompt,
CancelPrompt, RespondPermission, and Subscribe to preserve the existing caller API.
ConversationService owns prompt execution, permission interactions, journal storage,
subscriptions, and conversation state. RuntimeConversation names the optional
provider capability, distinct from this application service.

The conversation receives its context and storage path at construction and binds
to a configuration snapshot and process after the single successful launch. It
holds no reference to RunController and never launches or closes the process.
Controller status combines launch state, conversation state, and process liveness.

ConversationService owns mu and turnMu. mu serializes prompt admission, durable
append, publication, and interaction state; turnMu serializes prompt acceptance,
completion, and cancellation delivery. Acquire turnMu before conversation mu.
Controller lifecycle operations may acquire controller mu before conversation mu;
conversation operations never acquire controller mu or call back into it.

Shutdown first fences conversation admission and cancels its context, then closes
the current process and joins an in-flight launch. A process returned by a late
launch is also closed. The controller finally joins accepted prompt work before
closing the journal and subscriptions. It does not hold its lifecycle lock while
waiting for conversation work. Closing a conversation alone does not close its
provider process. No accepted work belongs to an HTTP request or stream lifetime.

import { Link, useParams } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft } from 'lucide-react';
import { useApp } from '../../app/context';
import { agentsQuery, directChatsQuery } from '../../shared/api/queries';
import { ErrorState, Loading, PageHeader } from '../../shared/ui/common';
import { TaskChat } from '../tasks/task-chat';
import type { Task } from '../../shared/api/generated/types.gen';

export function DirectChatPage() {
  const { scope } = useApp();
  const { conversationId } = useParams({ strict: false });
  const conversations = useQuery(directChatsQuery(scope));
  const agents = useQuery(agentsQuery(scope));
  if (conversations.isPending || agents.isPending) return <Loading />;
  if (conversations.isError) return <ErrorState error={conversations.error} />;
  if (agents.isError) return <ErrorState error={agents.error} />;
  const conversation = conversations.data.find((item) => item.id === conversationId);
  const agent = agents.data.find((item) => item.id === conversation?.agentId);
  if (!conversation || !agent) return <ErrorState error={new Error('This conversation is unavailable.')} />;
  const chatTask: Task = {
    id: conversation.id,
    workspaceId: scope.workspaceID,
    title: agent.name,
    description: agent.description,
    sourceProvider: '',
    status: 'in_progress',
    assigneeAgentId: agent.id,
    labels: [],
    repositories: [],
    createdAt: conversation.createdAt,
    updatedAt: conversation.createdAt,
  };
  return (
    <div className="page">
      <PageHeader title={agent.name} description="Direct conversation with this workspace agent.">
        <Link className="button button-outline button-sm" to="/organizations/$orgId/workspaces/$workspaceId/agents" params={{ orgId: scope.orgID, workspaceId: scope.workspaceID }}>
          <ArrowLeft size={14} /> Agents
        </Link>
      </PageHeader>
      <TaskChat task={chatTask} direct />
    </div>
  );
}

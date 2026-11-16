import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import {
  Activity,
  Building2,
  Boxes,
  Check,
  Copy,
  LoaderCircle,
  PanelsTopLeft,
  RefreshCw,
  Shield,
  Users,
} from 'lucide-react';
import { meQuery } from '../../shared/api/queries';
import { command, data } from '../../shared/api/client';
import * as sdk from '../../shared/api/generated/sdk.gen';
import { Button } from '../../shared/ui/button';
import { ErrorState, Loading } from '../../shared/ui/common';
import './admin.css';

const overviewQuery = {
  queryKey: ['platform-admin', 'overview'],
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    data(sdk.getPlatformAdminOverview({ ...command, signal })),
  staleTime: 60_000,
};
const configurationQuery = {
  queryKey: ['platform-admin', 'runtime-configuration'],
  queryFn: ({ signal }: { signal: AbortSignal }) =>
    data(sdk.getPlatformRuntimeConfiguration({ ...command, signal })),
  staleTime: 60_000,
};
const numberFormat = new Intl.NumberFormat();

function CopyValue({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  const [failed, setFailed] = useState(false);
  async function copy() {
    setCopied(false);
    setFailed(false);
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
    } catch {
      setFailed(true);
    }
  }
  return (
    <span className="admin-copy-value">
      <code>{value}</code>
      <Button
        variant="ghost"
        size="icon"
        className="admin-copy-button"
        onClick={() => void copy()}
        aria-label={copied ? `${label} copied` : `Copy ${label}`}
        title={copied ? 'Copied to clipboard' : `Copy ${label}`}
      >
        {copied ? <Check size={14} /> : <Copy size={14} />}
      </Button>
      {(copied || failed) && (
        <span className="admin-copy-feedback" role="status">
          {copied ? 'Copied' : 'Clipboard unavailable'}
        </span>
      )}
    </span>
  );
}

export function PlatformAdminPage() {
  const me = useQuery(meQuery);
  const overview = useQuery({ ...overviewQuery, enabled: me.data?.platformAdmin === true });
  const configuration = useQuery({
    ...configurationQuery,
    enabled: me.data?.platformAdmin === true,
  });
  if (me.isPending) return <Loading />;
  if (me.isError) return <ErrorState error={me.error} />;
  if (!me.data.platformAdmin)
    return <div className="notice error">Platform administrator access required.</div>;
  const refreshing = overview.isFetching || configuration.isFetching;
  const refresh = () => {
    void overview.refetch();
    void configuration.refetch();
  };
  const counts = [
    {
      label: 'Enabled accounts',
      value: overview.data?.enabledUsers,
      caption: 'Provisioned users',
      icon: Users,
    },
    {
      label: 'Organizations',
      value: overview.data?.organizations,
      caption: 'Across the platform',
      icon: Building2,
    },
    {
      label: 'Workspaces',
      value: overview.data?.workspaces,
      caption: 'Across all organizations',
      icon: PanelsTopLeft,
    },
    {
      label: 'Runs · last 30 days',
      value: overview.data?.runsLast30Days,
      caption: 'Rolling 30-day window',
      icon: Activity,
    },
  ] as const;
  const auth = configuration.data?.codexAuth;
  return (
    <>
      <header className="page-header admin-page-header">
        <div className="admin-page-title">
          <span className="admin-page-icon">
            <Shield size={20} />
          </span>
          <div>
            <p className="eyebrow">PLATFORM ADMINISTRATION</p>
            <h1>Platform overview</h1>
            <p className="muted">
              A snapshot of Orkest usage and the configuration this server is running.
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          onClick={refresh}
          disabled={refreshing}
          aria-label={refreshing ? 'Refreshing dashboard data' : 'Refresh dashboard data'}
          title={refreshing ? 'Refreshing dashboard data' : 'Refresh dashboard data'}
        >
          {refreshing ? <LoaderCircle className="spin" size={15} /> : <RefreshCw size={15} />}
          <span className="admin-refresh-label">{refreshing ? 'Refreshing' : 'Refresh data'}</span>
        </Button>
      </header>
      <section className="admin-count-grid" aria-label="Platform analytics">
        {counts.map(({ label, value, caption, icon: Icon }) => (
          <article className="panel admin-count-card" key={label} aria-busy={overview.isPending}>
            <div className="admin-count-heading">
              <span className="admin-count-icon">
                <Icon size={17} />
              </span>
              <span>{label}</span>
            </div>
            <strong>
              {value === undefined ? (
                <span className="admin-count-placeholder" aria-label="Loading">
                  —
                </span>
              ) : (
                numberFormat.format(value)
              )}
            </strong>
            <span className="muted admin-count-caption">{caption}</span>
          </article>
        ))}
      </section>
      {overview.isError && (
        <ErrorState error={overview.error} retry={() => void overview.refetch()} />
      )}
      {overview.data && (
        <p className="muted admin-updated">
          Analytics updated {new Date(overview.dataUpdatedAt).toLocaleTimeString()}
        </p>
      )}
      <section className="panel admin-runtime">
        <div className="panel-heading">
          <div>
            <h2>
              <Boxes size={18} /> Runtime configuration
            </h2>
            <p className="muted">
              Effective settings for this server process. These values are read-only here.
            </p>
          </div>
          {configuration.data && (
            <span className="admin-executor-badge">
              <span />
              {configuration.data.executor} executor
            </span>
          )}
        </div>
        {configuration.isError ? (
          <ErrorState error={configuration.error} retry={() => void configuration.refetch()} />
        ) : configuration.isPending ? (
          <div className="admin-config-loading" role="status">
            <LoaderCircle className="spin" size={18} /> Loading runtime configuration…
          </div>
        ) : (
          <>
            <dl className="admin-config-list">
              <div>
                <dt>Built-in agent MCP URL</dt>
                <dd>
                  <CopyValue label="MCP URL" value={configuration.data.agentMcpUrl} />
                  <span className="admin-source-label">
                    Source <b>{configuration.data.agentMcpSource.replaceAll('_', ' ')}</b>
                  </span>
                </dd>
              </div>
              <div>
                <dt>Data directory</dt>
                <dd>
                  <CopyValue label="data directory" value={configuration.data.dataDirectory} />
                  <span className="admin-config-hint">Durable platform and recovery data</span>
                </dd>
              </div>
              <div>
                <dt>Codex authentication</dt>
                <dd>
                  {auth?.configured ? (
                    <>
                      <span className="admin-config-value">{auth.type.replaceAll('_', ' ')}</span>
                      {auth.location && (
                        <CopyValue label="Codex auth location" value={auth.location} />
                      )}
                    </>
                  ) : (
                    <span className="admin-config-value admin-config-muted">Not configured</span>
                  )}
                </dd>
              </div>
              <div className="admin-images">
                <dt>Agentctl images</dt>
                <dd>
                  {Object.entries(configuration.data.agentctlImages).map(([runtime, image]) => (
                    <span key={runtime}>
                      <b>{runtime.replaceAll('-', ' ')}</b>
                      <CopyValue label={`${runtime} image`} value={image} />
                    </span>
                  ))}
                </dd>
              </div>
            </dl>
            <p className="muted admin-loaded">
              Configuration loaded {new Date(configuration.data.loadedAt).toLocaleString()}
            </p>
          </>
        )}
      </section>
    </>
  );
}

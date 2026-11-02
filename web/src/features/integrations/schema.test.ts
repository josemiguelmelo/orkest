import { describe, it, expect } from 'vitest';
import { githubProvider } from '../../test/fixtures';
import { integrationBody, providerFields, validateConfig } from './schema';
describe('provider configuration', () => {
  it('renders and validates published provider constraints', () => {
    expect(providerFields(githubProvider.configSchema).required).toEqual(['projectUrl']);
    expect(
      validateConfig(githubProvider.configSchema, {
        projectUrl: 'https://github.com/orgs/acme/projects/1',
      }),
    ).toEqual({ projectUrl: 'https://github.com/orgs/acme/projects/1' });
    expect(() =>
      validateConfig(githubProvider.configSchema, { projectUrl: 'https://example.test/project' }),
    ).toThrow('configuration');
    expect(() =>
      validateConfig(githubProvider.configSchema, {
        projectUrl: 'https://github.com/orgs/acme/projects/1',
        extra: true,
      }),
    ).toThrow();
  });
  it('supports Jira string-array fields', () => {
    const schema = {
      type: 'object',
      properties: {
        baseUrl: { type: 'string', format: 'uri' },
        projects: { type: 'array', items: { type: 'string' }, minItems: 1, uniqueItems: true },
      },
      required: ['baseUrl', 'projects'],
      additionalProperties: false,
    };
    expect(providerFields(schema).properties.projects?.type).toBe('array');
    expect(() =>
      validateConfig(schema, { baseUrl: 'https://jira.example.test', projects: [] }),
    ).toThrow();
  });
  it('fails explicitly for unsupported schemas', () => {
    expect(() =>
      providerFields({ type: 'object', properties: { count: { type: 'number' } } }),
    ).toThrow('cannot render');
    expect(() =>
      providerFields({ type: 'object', properties: { token: { type: 'string', oneOf: [] } } }),
    ).toThrow();
  });
  it('omits unchanged credentials instead of overwriting them', () => {
    const values = {
      workflowId: 'workflow-1',
      provider: 'github_cloud',
      name: 'GitHub',
      enabled: true,
      labels: 'frontend, urgent',
      interval: '5m',
      credential: '',
    };
    expect(integrationBody(values, {})).not.toHaveProperty('credential');
    expect(integrationBody({ ...values, credential: 'temporary' }, {}).credential).toBe(
      'temporary',
    );
  });
});

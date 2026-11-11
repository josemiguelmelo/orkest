import { describe, it, expect } from 'vitest';
import { argumentInput, headerInput, stdioInput, mcpSchema } from './mcp-model';
describe('write-only MCP headers', () => {
  it('distinguishes preserve, clear and replace', () => {
    expect(headerInput('preserve', '')).toEqual({});
    expect(headerInput('clear', '')).toEqual({ headers: {} });
    expect(headerInput('replace', '{"Authorization":"Bearer temporary"}')).toEqual({
      headers: { Authorization: 'Bearer temporary' },
    });
  });
  it('rejects malformed headers without displaying their values', () => {
    expect(() => headerInput('replace', '{"Invalid Header":"secret"}')).toThrow();
    expect(
      mcpSchema.safeParse({
        name: 'MCP',
        url: 'https://example.test/mcp',
        mode: 'replace',
        headers: '{"Authorization":5}',
      }).success,
    ).toBe(false);
  });
  it('rejects credentials and executable URL schemes', () => {
    for (const url of ['javascript:alert(1)', 'https://user:secret@example.test/mcp'])
      expect(mcpSchema.safeParse({ name: 'MCP', url, mode: 'preserve', headers: '' }).success).toBe(
        false,
      );
  });
});

describe('stdio MCP configuration', () => {
  it('accepts an executable and literal arguments without requiring a URL', () => {
    expect(
      mcpSchema.safeParse({
        name: 'Tools',
        transport: 'stdio',
        command: '/opt/mcp/server',
        args: '["literal space", "$TOKEN"]',
        url: '',
        mode: 'preserve',
        headers: '',
      }).success,
    ).toBe(true);
  });
  it('rejects relative executables, invalid argument arrays and invalid environment names', () => {
    const valid = {
      name: 'Tools',
      transport: 'stdio',
      command: '/opt/mcp/server',
      args: '[]',
      url: '',
      mode: 'preserve',
      headers: '',
    };
    for (const patch of [
      { command: 'server' },
      { args: '[5]' },
      { args: '["bad\\u0000arg"]' },
      { mode: 'replace', headers: '{"BAD-NAME":"secret"}' },
    ])
      expect(mcpSchema.safeParse({ ...valid, ...patch }).success).toBe(false);
  });
  it('preserves, clears and replaces write-only environment values', () => {
    expect(stdioInput('preserve', '')).toEqual({});
    expect(stdioInput('clear', '')).toEqual({ environment: {} });
    expect(stdioInput('replace', '{"TOKEN":"private"}')).toEqual({
      environment: { TOKEN: 'private' },
    });
    expect(argumentInput('["literal space", "$TOKEN"]')).toEqual(['literal space', '$TOKEN']);
  });
});

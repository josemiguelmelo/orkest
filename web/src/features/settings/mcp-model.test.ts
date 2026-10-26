import { describe, it, expect } from 'vitest';
import { headerInput, mcpSchema } from './mcp-model';
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

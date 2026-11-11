import { z } from 'zod';
const headersSchema = z
  .record(
    z
      .string()
      .regex(/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/)
      .max(256),
    z
      .string()
      .max(8192)
      .refine((value) => !/[\r\n]/.test(value)),
  )
  .refine((value) => Object.keys(value).length <= 50);
export function headerInput(
  mode: 'preserve' | 'replace' | 'clear',
  text: string,
): { headers?: Record<string, string> } {
  if (mode === 'preserve') return {};
  if (mode === 'clear') return { headers: {} };
  const value: unknown = JSON.parse(text);
  return { headers: headersSchema.parse(value) };
}
const environmentSchema = z
  .record(
    z
      .string()
      .regex(/^[A-Za-z_][A-Za-z0-9_]*$/)
      .max(256),
    z
      .string()
      .max(8192)
      .refine((value) => !value.includes('\0')),
  )
  .refine((value) => Object.keys(value).length <= 50);
const argsSchema = z
  .array(
    z
      .string()
      .max(8192)
      .refine((value) => !value.includes('\0')),
  )
  .max(100);
export function stdioInput(mode: 'preserve' | 'replace' | 'clear', text: string) {
  if (mode === 'preserve') return {};
  if (mode === 'clear') return { environment: {} };
  return { environment: environmentSchema.parse(JSON.parse(text) as unknown) };
}
export function argumentInput(text: string): string[] {
  return argsSchema.parse(JSON.parse(text) as unknown);
}
export const mcpSchema = z
  .object({
    name: z.string().trim().min(1).max(200),
    transport: z.enum(['http', 'stdio']),
    url: z.string(),
    command: z.string().default(''),
    args: z.string().default('[]'),
    mode: z.enum(['preserve', 'replace', 'clear']),
    headers: z.string(),
  })
  .superRefine((values, ctx) => {
    if (values.transport === 'http') {
      try {
        const u = new URL(values.url);
        if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password || u.hash)
          throw new Error();
      } catch {
        ctx.addIssue({
          code: 'custom',
          path: ['url'],
          message: 'Enter an HTTP(S) URL without credentials or fragment.',
        });
      }
    } else {
      if (
        !values.command.startsWith('/') ||
        /[\0\r\n]/.test(values.command) ||
        values.command.length > 4096
      )
        ctx.addIssue({
          code: 'custom',
          path: ['command'],
          message: 'Enter an absolute executable path in the execution environment.',
        });
      try {
        argumentInput(values.args);
      } catch {
        ctx.addIssue({
          code: 'custom',
          path: ['args'],
          message: 'Enter a JSON array of strings (maximum 100 arguments).',
        });
      }
    }
    if (values.mode === 'replace') {
      try {
        if (values.transport === 'stdio') stdioInput(values.mode, values.headers);
        else headerInput(values.mode, values.headers);
      } catch {
        ctx.addIssue({
          code: 'custom',
          path: ['headers'],
          message: 'Enter a JSON object with valid names and string values (maximum 50 entries).',
        });
      }
    }
  });

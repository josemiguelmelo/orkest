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
export const mcpSchema = z
  .object({
    name: z.string().trim().min(1).max(200),
    url: z.url().refine((value) => {
      const u = new URL(value);
      return ['http:', 'https:'].includes(u.protocol) && !u.username && !u.password && !u.hash;
    }, 'Enter an HTTP(S) URL without credentials or fragment.'),
    mode: z.enum(['preserve', 'replace', 'clear']),
    headers: z.string(),
  })
  .superRefine((values, ctx) => {
    if (values.mode === 'replace') {
      try {
        headerInput(values.mode, values.headers);
      } catch {
        ctx.addIssue({
          code: 'custom',
          path: ['headers'],
          message:
            'Enter a JSON object with valid header names and string values (maximum 50 headers).',
        });
      }
    }
  });

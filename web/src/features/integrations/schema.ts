import { z } from 'zod';
import Ajv2020 from 'ajv/dist/2020';
import addFormats from 'ajv-formats';

export type JsonValue =
  null | boolean | number | string | JsonValue[] | { [key: string]: JsonValue };
const jsonValue: z.ZodType<JsonValue> = z.lazy(() =>
  z.union([
    z.null(),
    z.boolean(),
    z.number(),
    z.string(),
    z.array(jsonValue),
    z.record(z.string(), jsonValue),
  ]),
);
export const configSchema = z.record(z.string(), jsonValue);
const fieldSchema = z
  .object({
    type: z.enum(['string', 'array']),
    description: z.string().optional(),
    title: z.string().optional(),
    format: z.string().optional(),
    pattern: z.string().optional(),
    minLength: z.number().optional(),
    maxLength: z.number().optional(),
    minItems: z.number().optional(),
    uniqueItems: z.boolean().optional(),
    items: z
      .object({ type: z.literal('string') })
      .strict()
      .optional(),
  })
  .strict();
const rendererSchema = z
  .object({
    $schema: z.string().optional(),
    $id: z.string().optional(),
    title: z.string().optional(),
    description: z.string().optional(),
    type: z.literal('object'),
    properties: z.record(z.string(), fieldSchema),
    required: z.array(z.string()).optional(),
    additionalProperties: z.literal(false).optional(),
  })
  .strict();
export type ProviderFields = z.infer<typeof rendererSchema>;
export function providerFields(schema: Record<string, unknown>): ProviderFields {
  const result = rendererSchema.safeParse(schema);
  if (!result.success)
    throw new Error(
      'This provider uses a configuration schema this webapp cannot render. Update the webapp to configure it.',
    );
  for (const field of Object.values(result.data.properties))
    if (field.type === 'array' && !field.items) throw new Error('Unsupported array configuration.');
  return result.data;
}
export function validateConfig(
  schema: Record<string, unknown>,
  input: unknown,
): Record<string, JsonValue> {
  const config = configSchema.parse(input);
  const ajv = new Ajv2020({ allErrors: true, strict: true });
  addFormats(ajv);
  const validate = ajv.compile(schema);
  if (!validate(config))
    throw new Error(
      `Check provider configuration: ${ajv.errorsText(validate.errors, { separator: '; ' })}`,
    );
  return config;
}
export function integrationBody(
  values: {
    provider: string;
    name: string;
    enabled: boolean;
    labels: string;
    interval: string;
    credential: string;
  },
  config: Record<string, JsonValue>,
) {
  return {
    provider: values.provider,
    name: values.name,
    enabled: values.enabled,
    labels: values.labels
      .split(',')
      .map((v) => v.trim())
      .filter(Boolean),
    interval: values.interval,
    config,
    ...(values.credential ? { credential: values.credential } : {}),
  };
}

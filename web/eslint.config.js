import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import reactHooks from 'eslint-plugin-react-hooks';

export default tseslint.config(
  { ignores:['dist/**','src/shared/api/generated/**','.api-source/**','.api-check/**','node_modules/**','playwright-report/**','test-results/**'] },
  { files:['src/**/*.{ts,tsx}'], extends:[js.configs.recommended,...tseslint.configs.recommendedTypeChecked], languageOptions:{parserOptions:{project:'./tsconfig.json',tsconfigRootDir:import.meta.dirname}}, plugins:{'react-hooks':reactHooks}, rules:{...reactHooks.configs.recommended.rules,'@typescript-eslint/no-explicit-any':'error','@typescript-eslint/no-floating-promises':'error','@typescript-eslint/no-misused-promises':['error',{checksVoidReturn:{attributes:false}}],'@typescript-eslint/consistent-type-imports':'error','@typescript-eslint/no-unused-vars':['error',{argsIgnorePattern:'^_'}],'react-hooks/incompatible-library':'off'} },
);

'use strict';

// ESLint flat config (ESLint 10 reads only this format). The tools and their
// pinned versions live in tools/lint; see `task lint-web`.
//
// The repo has no root package.json, so the config packages are resolved from
// tools/lint/node_modules rather than from this directory.
const { createRequire } = require('node:module');
const path = require('node:path');

const toolRequire = createRequire(path.join(__dirname, 'tools', 'lint', 'package.json'));
const js = toolRequire('@eslint/js');
const globals = toolRequire('globals');

// Switch every browser global off, for the Node-hosted test directories.
const noBrowser = Object.fromEntries(Object.keys(globals.browser).map((name) => [name, 'off']));

module.exports = [
  js.configs.recommended,
  {
    linterOptions: { reportUnusedDisableDirectives: 'off' },
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'script',
      globals: { ...globals.browser, ...globals.es2022 },
    },
    rules: {
      'no-unused-vars': ['warn', { args: 'none', ignoreRestSiblings: true }],
      'no-undef': 'error',
      'no-console': ['off'],
      complexity: ['warn', 8],
      'max-lines-per-function': ['warn', { max: 60, skipComments: true, skipBlankLines: true }],
      'max-depth': ['warn', 4],
      'no-var': 'error',
      'prefer-const': ['warn', { destructuring: 'any', ignoreReadBeforeAssign: true }],
      'prefer-arrow-callback': 'off',
      'func-style': ['off'],
      'no-shadow': ['warn', { builtinGlobals: false, hoist: 'functions', allow: [] }],
      'no-redeclare': ['error', { builtinGlobals: false }],
      'no-useless-escape': 'off',
      'no-restricted-globals': 'off',
      // Added to eslint:recommended in ESLint 10. Off to keep the ruleset
      // identical to the ESLint 9 / .eslintrc.json one; enable them (and fix
      // the findings) deliberately in a separate change.
      'no-unassigned-vars': 'off',
      'no-useless-assignment': 'off',
      'preserve-caught-error': 'off',
    },
  },
  {
    files: ['test/js/**/*.js', 'test/interop/**/*.js', 'test/e2e/**/*.js'],
    languageOptions: {
      globals: { ...noBrowser, ...globals.node, window: 'readonly' },
    },
    rules: {
      complexity: 'off',
      'max-lines-per-function': 'off',
    },
  },
  {
    files: ['web/js/**/*.js', 'web/dev/**/*.js'],
    rules: {
      'no-restricted-properties': 'off',
    },
  },
];

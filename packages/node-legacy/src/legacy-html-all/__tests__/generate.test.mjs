import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { loadGenerator } from '@doc-kit/core/generators/loader.mjs';
import { setConfig } from '@doc-kit/core/utils/configuration/index.mjs';
import { loadMarkdownPlugins } from '@doc-kit/core/utils/markdown/plugins.mjs';

import { generate } from '../generate.mjs';

// The navigation is rendered with the pipeline of `legacy-html`
await loadMarkdownPlugins(
  await loadGenerator(import.meta.resolve('../../legacy-html/index.mjs'))
);

const config = await setConfig({
  target: ['legacy-html-all'],
  version: 'v22.0.0',
});

const page = (api, section) => ({
  api,
  path: `/${api}`,
  section,
  toc: '',
  nav: '',
  content: `<section>${section}</section>`,
});

describe('legacy-html-all', () => {
  it('links every module in the side navigation', async () => {
    config['legacy-html-all'].output = undefined;

    const result = await generate([
      page('index', 'Index'),
      page('assert', 'Assert'),
      page('fs', 'File system'),
    ]);

    assert.match(
      result,
      /<a class="nav-assert" href="assert\.html">Assert<\/a>/
    );
    assert.match(result, /<a class="nav-fs" href="fs\.html">File system<\/a>/);
    assert.doesNotMatch(result, /class="nav-index"/);
  });
});

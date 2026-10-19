import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { replaceTemplateValues } from '../replaceTemplateValues.mjs';

const config = {
  // Only the version string is read
  version: { version: '22.0.0' },
  changelog: [],
  pageURL: '{baseURL}/latest-{version}/api{path}.html',
  editURL: 'https://github.com/nodejs/node/edit/main/doc/api{path}.md',
};

const values = {
  api: 'addons',
  path: '/addons',
  added: '',
  section: 'C++ addons',
  toc: '',
  nav: '',
};

describe('replaceTemplateValues', () => {
  it('fills the template', () => {
    const result = replaceTemplateValues(
      '<title>__SECTION__ | __VERSION__</title><main>__CONTENT__</main>',
      { ...values, content: '<p>Hello</p>' },
      config
    );

    assert.equal(
      result,
      '<title>C++ addons | v22.0.0</title><main><p>Hello</p></main>'
    );
  });

  it('inserts values literally, `$` patterns included', () => {
    const content =
      '<pre>dirname "$$(command -v node)" -o $@ $&#x3C; $` $\'</pre>';

    const result = replaceTemplateValues(
      '<main>__CONTENT__</main>',
      { ...values, content },
      config
    );

    assert.equal(result, `<main>${content}</main>`);
  });
});

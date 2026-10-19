import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { u } from 'unist-builder';

import { setConfig } from '#utils/configuration/index.mjs';

import { generate } from '../generate.mjs';

const config = await setConfig({ target: ['json-simple'] });

describe('json-simple', () => {
  it('drops headings and stability indexes from a copy of the entries', async () => {
    config['json-simple'].output = undefined;

    const entry = {
      api: 'fs',
      content: u('root', [
        u('heading', { depth: 1 }, [u('text', 'File system')]),
        u('blockquote', [u('paragraph', [u('text', 'Stability: 2 - Stable')])]),
        u('paragraph', [u('text', 'Body.')]),
      ]),
    };

    const before = structuredClone(entry);
    const [result] = await generate([entry]);

    assert.deepEqual(
      result.content.children.map(({ type }) => type),
      ['paragraph']
    );

    // Other generators of the run read the same entries
    assert.deepEqual(entry, before);
  });
});

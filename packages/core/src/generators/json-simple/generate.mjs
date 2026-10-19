'use strict';

import { join } from 'node:path';

import { remove } from 'unist-util-remove';

import getConfig from '#utils/configuration/index.mjs';
import { writeJSON } from '#utils/file.mjs';
import { UNIST } from '#utils/queries/index.mjs';

/**
 * Generates the simplified JSON version of the API docs
 *
 * @type {import('./types').Generator['generate']}
 */
export async function generate(input) {
  const config = getConfig('json-simple');

  // The entries are shared with the other generators of the run, so the
  // nodes are removed from a copy
  const entries = structuredClone(input);

  entries.forEach(node =>
    remove(node.content, [UNIST.isStabilityNode, UNIST.isHeading])
  );

  if (config.output) {
    await writeJSON(
      join(config.output, 'api-docs.json'),
      entries,
      config.minify
    );
  }

  return entries;
}

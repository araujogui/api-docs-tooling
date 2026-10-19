import { deepStrictEqual, ok, strictEqual } from 'node:assert';
import { describe, it } from 'node:test';

import { loadGenerator } from '../../generators/loader.mjs';
import createWorkerPool from '../index.mjs';
import createParallelWorker from '../parallel.mjs';

const metadataGenerator = await loadGenerator('metadata');
const astJsGenerator = await loadGenerator('ast-js');

/**
 * Helper to collect all results from an async generator.
 *
 * @template T
 * @param {AsyncGenerator<T[], void, unknown>} generator
 * @returns {Promise<T[]>}
 */
async function collectStream(generator) {
  const results = [];

  for await (const chunk of generator) {
    results.push(...chunk);
  }

  return results;
}

/**
 * Helper to collect chunks (not flattened)
 *
 * @template T
 * @param {AsyncGenerator<T[], void, unknown>} generator
 * @returns {Promise<T[][]>}
 */
async function collectChunks(generator) {
  const chunks = [];

  for await (const chunk of generator) {
    chunks.push(chunk);
  }

  return chunks;
}

describe('createParallelWorker', () => {
  it('should create a ParallelWorker with stream method', async () => {
    const pool = createWorkerPool(2);
    const worker = createParallelWorker('metadata', metadataGenerator, pool, {
      threads: 2,
    });

    ok(worker);
    strictEqual(typeof worker.stream, 'function');

    await pool.destroy();
  });

  it('should handle empty items array', async () => {
    const pool = createWorkerPool(2);
    const worker = createParallelWorker('ast-js', astJsGenerator, pool, {
      threads: 2,
      chunkSize: 10,
    });

    const results = await collectStream(worker.stream([], [], {}));

    deepStrictEqual(results, []);

    await pool.destroy();
  });

  it('should distribute items to multiple worker threads', async () => {
    const pool = createWorkerPool(4);
    const worker = createParallelWorker('metadata', metadataGenerator, pool, {
      threads: 4,
      chunkSize: 1,
    });

    const mockInput = [
      {
        path: 'test1',
        tree: { type: 'root', children: [] },
      },
      {
        path: 'test2',
        tree: { type: 'root', children: [] },
      },
      {
        path: 'test3',
        tree: { type: 'root', children: [] },
      },
      {
        path: 'test4',
        tree: { type: 'root', children: [] },
      },
    ];

    const chunks = await collectChunks(worker.stream(mockInput, {}));

    strictEqual(chunks.length, 4);

    for (const chunk of chunks) {
      ok(Array.isArray(chunk));
    }

    await pool.destroy();
  });

  it('should yield results as chunks complete', async () => {
    const pool = createWorkerPool(2);
    const worker = createParallelWorker('metadata', metadataGenerator, pool, {
      threads: 2,
      chunkSize: 1,
    });

    const mockInput = [
      {
        path: 'test1',
        tree: { type: 'root', children: [] },
      },
      {
        path: 'test2',
        tree: { type: 'root', children: [] },
      },
    ];

    const chunks = await collectChunks(worker.stream(mockInput, {}));

    strictEqual(chunks.length, 2);

    await pool.destroy();
  });

  it('should yield results in the order of the items', async () => {
    // Later chunks complete first
    const generator = {
      name: 'ordered',
      processChunk: async input => {
        await new Promise(resolve => setTimeout(resolve, (3 - input[0]) * 20));

        return input;
      },
    };

    const worker = createParallelWorker('ordered', generator, undefined, {
      threads: 1,
      chunkSize: 1,
    });

    const chunks = await collectChunks(worker.stream([0, 1, 2], {}));

    deepStrictEqual(chunks, [[0], [1], [2]]);
  });

  it('should give single-threaded chunks their own copy of the items', async () => {
    const generator = {
      name: 'editing',
      processChunk: async input => {
        input[0].edited = true;

        return input;
      },
    };

    const items = [{}, {}, {}];
    const worker = createParallelWorker('editing', generator, undefined, {
      threads: 1,
      chunkSize: 1,
    });

    await collectChunks(worker.stream(items, {}));

    deepStrictEqual(items, [{}, {}, {}]);
  });

  it('should work with single thread and items', async () => {
    const pool = createWorkerPool(2);
    const worker = createParallelWorker('metadata', metadataGenerator, pool, {
      threads: 2,
      chunkSize: 5,
    });

    const mockInput = [
      {
        path: 'test1',
        tree: { type: 'root', children: [] },
      },
    ];

    const chunks = await collectChunks(worker.stream(mockInput, {}));

    strictEqual(chunks.length, 1);
    ok(Array.isArray(chunks[0]));

    await pool.destroy();
  });

  it('should use sliceInput for metadata generator', async () => {
    const pool = createWorkerPool(2);
    const worker = createParallelWorker('metadata', metadataGenerator, pool, {
      threads: 2,
      chunkSize: 1,
    });

    const mockInput = [
      {
        path: 'test1',
        tree: { type: 'root', children: [] },
      },
      {
        path: 'test2',
        tree: { type: 'root', children: [] },
      },
    ];

    const chunks = await collectChunks(worker.stream(mockInput, {}));

    strictEqual(chunks.length, 2);

    await pool.destroy();
  });
});

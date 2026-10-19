'use strict';

import logger from '#logger/index.mjs';

const parallelLogger = logger.child('parallel');

/**
 * Splits items into chunks of specified size.
 *
 * @param {number} count - Total number of items
 * @param {number} size - Maximum items per chunk
 * @returns {number[][]} Array of index arrays for each chunk
 */
const createChunks = (count, size) => {
  const chunks = [];

  for (let i = 0; i < count; i += size) {
    chunks.push(
      Array.from({ length: Math.min(size, count - i) }, (_, j) => i + j)
    );
  }

  return chunks;
};

/**
 * Prepares task data for a chunk, slicing input to only include relevant items.
 *
 * @param {unknown[]} fullInput - Full input array
 * @param {number[]} indices - Indices to process
 * @param {Object} extra - Stuff to pass to the worker
 * @param {import('../utils/configuration/types').Configuration} configuration - Serialized options
 * @param {string} generatorSpecifier - Resolved specifier of the generator
 * @param {string} generatorName - Name of the generator
 * @returns {ParallelTaskOptions} Task data for Piscina
 */
const createTask = (
  fullInput,
  indices,
  extra,
  configuration,
  generatorSpecifier,
  generatorName
) => {
  return {
    generatorSpecifier,
    // Only send the items needed for this chunk (reduces serialization overhead)
    input: indices.map(i => fullInput[i]),
    // Remap indices to 0-based for the sliced array
    itemIndices: indices.map((_, i) => i),
    extra,
    // Only pass the needed configuration to this generator
    configuration: {
      [generatorName]: configuration[generatorName],
    },
  };
};

/**
 * Creates a parallel worker that distributes work across a Piscina thread pool.
 *
 * @param {string} specifier - Resolved generator specifier (importable from workers)
 * @param {GeneratorMetadata} generator - The loaded generator
 * @param {import('piscina').Piscina} pool - Piscina instance
 * @param {import('../utils/configuration/types').Configuration} configuration - Generator options
 * @returns {ParallelWorker}
 */
export default function createParallelWorker(
  specifier,
  generator,
  pool,
  configuration
) {
  const { threads, chunkSize } = configuration;

  const { name } = generator;

  return {
    /**
     * Processes items in parallel, yielding each chunk's results in the order
     * of the items.
     *
     * @template T
     * @param {T[]} items - Items to process
     * @param {object} extra - Extra options
     * @yields {R[]} Chunk results, in the order of the items
     */
    async *stream(items, extra) {
      if (items.length === 0) {
        return;
      }

      const chunks = createChunks(items.length, chunkSize);

      parallelLogger.debug(
        `Distributing ${items.length} items across ${chunks.length} chunks`,
        { generator: name, chunks: chunks.length, chunkSize, threads }
      );

      const runInOneGo = threads <= 1 || items.length <= 2;

      // Submit all tasks at once, so they all run in parallel
      const tasks = chunks.map(indices =>
        runInOneGo
          ? // Like a worker, the chunk works on its own copy of its items, as
            // generators may edit them while other generators share them
            generator.processChunk(
              structuredClone(indices.map(i => items[i])),
              indices.map((_, i) => i),
              extra
            )
          : pool.run(
              createTask(items, indices, extra, configuration, specifier, name)
            )
      );

      // A chunk that fails before the ones ahead of it is reported when its
      // turn comes, not as an unhandled rejection
      tasks.forEach(task => task.catch(() => {}));

      // Yield the results in the order of the items, whatever order the chunks
      // complete in, so the output doesn't change from run to run
      for (const [index, task] of tasks.entries()) {
        const result = await task;

        parallelLogger.debug(`Chunk ${index + 1}/${chunks.length} completed`, {
          generator: name,
        });

        yield result;
      }
    },
  };
}

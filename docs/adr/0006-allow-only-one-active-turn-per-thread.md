# Allow only one active turn per thread

A `Thread` will permit only one active `Run`, `RunJSON`, or `RunStreamed` operation and return `ErrTurnInProgress` when another turn is attempted before the active one releases the thread. The SDK will not silently queue turns or reproduce the TypeScript implementation's thread-ID race; separate threads may run concurrently, and the shared `Client` will remain safe for concurrent use.

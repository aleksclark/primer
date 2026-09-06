// Package façade for the generated Studio authoring REST contract.
// Consumers must use this client rather than constructing raw /studio/v1 URLs.
import createOpenAPIClient from "openapi-fetch";
import type { paths } from "../generated/schema";
export type { components, paths, operations } from "../generated/schema";

export interface ClientOptions {
  baseUrl: string;
  token?: string;
  fetch?: typeof globalThis.fetch;
}

export type StudioClient = ReturnType<typeof createOpenAPIClient<paths>>;

export function createClient(options: ClientOptions): StudioClient {
  const client = createOpenAPIClient<paths>({
    baseUrl: options.baseUrl,
    fetch: options.fetch,
  });

  if (options.token) {
    client.use({
      onRequest({ request }) {
        request.headers.set("Authorization", `Bearer ${options.token}`);
        return request;
      },
    });
  }

  return client;
}

import { studioClient } from './client';
import type { components } from './client';
export type Model<K extends keyof components['schemas']> = components['schemas'][K];
export class ApiFailure extends Error { constructor(public status: number, message: string) { super(message); } }
export function problem(error: unknown): string {
 if (error instanceof Error) return error.message;
 if (typeof error === 'object' && error && 'detail' in error) {
  const findings='errors' in error && Array.isArray(error.errors)
   ? error.errors.flatMap(f=>typeof f==='object'&&f&&'message' in f?[String(f.message)]:[])
   : [];
  return [String(error.detail),...findings].join(' ');
 }
 return 'The request failed. Reload and try again.';
}
async function value<T>(request: Promise<{data?: T; error?: unknown; response: Response}>): Promise<T> {
 const result = await request;
 if (!result.response.ok) throw new ApiFailure(result.response.status, problem(result.error));
 if (result.data === undefined) throw new Error('The server returned no result.');
 return result.data;
}
const credentials = 'include' as const;
const query = (offset: number) => ({ limit: 10, offset });
export const graph = (revisionId: string, signal?: AbortSignal) => value(studioClient.GET('/studio/v1/revisions/{revisionId}/graph', {credentials, signal, params:{path:{revisionId}}}));
export const revisions = (curriculumId: string, offset=0, signal?: AbortSignal) => value(studioClient.GET('/studio/v1/curricula/{curriculumId}/revisions',{credentials, signal, params:{path:{curriculumId},query:query(offset)}}));
export const diff = (revisionId: string, fromRevisionId: string, signal?: AbortSignal) => value(studioClient.GET('/studio/v1/revisions/{revisionId}/diff',{credentials,signal,params:{path:{revisionId},query:{fromRevisionId}}}));
export const approval = (revisionId: string, signal?: AbortSignal) => value(studioClient.GET('/studio/v1/revisions/{revisionId}/approval',{credentials,signal,params:{path:{revisionId}}}));
export const decide = (revisionId: string, decision: 'approved'|'rejected', contentFingerprint: string) => value(studioClient.POST('/studio/v1/revisions/{revisionId}/approval',{credentials,params:{path:{revisionId}},body:{decision,contentFingerprint}}));
export type CommentTarget = {kind:'node'|'item'; id:string; revisionId:string; title:string};
export function comments(t:CommentTarget,offset:number,signal?:AbortSignal) {
 return t.kind==='node'
 ? value(studioClient.GET('/studio/v1/revisions/{revisionId}/comments',{credentials,signal,params:{path:{revisionId:t.revisionId},query:{...query(offset),nodeId:t.id}}}))
 : value(studioClient.GET('/studio/v1/materialized-items/{itemId}/comments',{credentials,signal,params:{path:{itemId:t.id},query:query(offset)}}));
}
export function comment(t:CommentTarget,body:string){return t.kind==='node'
 ? value(studioClient.POST('/studio/v1/revisions/{revisionId}/comments',{credentials,params:{path:{revisionId:t.revisionId}},body:{nodeId:t.id,body}}))
 : value(studioClient.POST('/studio/v1/materialized-items/{itemId}/comments',{credentials,params:{path:{itemId:t.id}},body:{body}}));}
export const templates=(workspaceId:string,offset:number,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/workspaces/{workspaceId}/templates',{credentials,signal,params:{path:{workspaceId},query:query(offset)}}));
export const fromTemplate=(workspaceId:string,name:string,templateCode:string)=>value(studioClient.POST('/studio/v1/workspaces/{workspaceId}/curricula',{credentials,params:{path:{workspaceId}},body:{name,templateCode}}));
export const saveTemplate=(workspaceId:string,code:string,name:string,seed:Model<'TemplateSeed'>)=>value(studioClient.POST('/studio/v1/workspaces/{workspaceId}/templates',{credentials,params:{path:{workspaceId}},body:{code,name,briefType:'custom',seed}}));
export const library=(workspaceId:string,offset:number,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/workspaces/{workspaceId}/unit-library',{credentials,signal,params:{path:{workspaceId},query:query(offset)}}));
export const saveUnit=(workspaceId:string,revisionId:string,unitId:string,name:string)=>value(studioClient.POST('/studio/v1/workspaces/{workspaceId}/unit-library',{credentials,params:{path:{workspaceId}},body:{revisionId,unitId,name}}));
export const importUnit=(revisionId:string,entryId:string)=>value(studioClient.POST('/studio/v1/revisions/{revisionId}/unit-library/{entryId}',{credentials,params:{path:{revisionId,entryId}}}));
export const shares=(curriculumId:string,offset:number,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/curricula/{curriculumId}/shares',{credentials,signal,params:{path:{curriculumId},query:query(offset)}}));
export const share=(curriculumId:string,targetWorkspaceId:string)=>value(studioClient.POST('/studio/v1/curricula/{curriculumId}/shares',{credentials,params:{path:{curriculumId}},body:{targetWorkspaceId,permission:'read'}}));
export async function revoke(curriculumId:string,targetWorkspaceId:string){const r=await studioClient.DELETE('/studio/v1/curricula/{curriculumId}/shares/{targetWorkspaceId}',{credentials,params:{path:{curriculumId,targetWorkspaceId}}});if(!r.response.ok)throw new ApiFailure(r.response.status,problem(r.error));}
export const policy=(workspaceId:string,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/workspaces/{workspaceId}/collaboration-policy',{credentials,signal,params:{path:{workspaceId}}}));
export const setPolicy=(workspaceId:string,body:Model<'CollaborationPolicy'>)=>value(studioClient.PUT('/studio/v1/workspaces/{workspaceId}/collaboration-policy',{credentials,params:{path:{workspaceId}},body}));
export const runs=(revisionId:string,offset:number,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/revisions/{revisionId}/materializations',{credentials,signal,params:{path:{revisionId},query:query(offset)}}));
export const items=(materializationId:string,offset:number,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/materializations/{materializationId}/items',{credentials,signal,params:{path:{materializationId},query:query(offset)}}));
export const addNode=(revisionId:string,kind:'unit'|'outcome',title:string,standardCode:string)=>value(studioClient.POST('/studio/v1/revisions/{revisionId}/nodes',{credentials,params:{path:{revisionId}},body:{kind,title,standardCodes:kind==='outcome'?[standardCode]:[],attributes:kind==='outcome'?{evidenceKind:'portfolio',evidenceDescription:'Demonstrate mastery'}:{}}}));
export const visibleCatalogs=(workspaceId:string,offset:number,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/workspaces/{workspaceID}/standards-catalogs',{credentials,signal,params:{path:{workspaceID:workspaceId},query:query(offset)}}));
export const catalogStandards=(catalogId:string,offset:number,q:string,signal?:AbortSignal)=>value(studioClient.GET('/studio/v1/standards-catalogs/{catalogId}/standards',{credentials,signal,params:{path:{catalogId},query:{...query(offset),q}}}));

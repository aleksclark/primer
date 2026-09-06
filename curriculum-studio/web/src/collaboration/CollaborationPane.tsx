import { useEffect, useState } from 'react';
import { addNode, graph, revisions, problem } from '../api/collaboration';
import type { CommentTarget, Model } from '../api/collaboration';
import { canAuthor, Notice, Pager, sameWorkspace, useRemote } from './common';
import CommentInspector from './CommentInspector';
import MaterializedComments from './MaterializedComments';
import { Review, RevisionDiff } from './Review';
import { UnitLibrary, Sharing } from './LibraryAndSharing';
import OutcomeStandardPicker from './OutcomeStandardPicker';

export default function CollaborationPane({workspaceId,role,curriculum,revision,onRevision}:{workspaceId:string;role:string;curriculum:Model<'Curriculum'>;revision:Model<'PlanRevision'>;onRevision:(r:Model<'PlanRevision'>)=>void}){
 const [offset,setOffset]=useState(0);const [from,setFrom]=useState('');
 const page=useRemote(signal=>revisions(curriculum.id,offset,signal),[curriculum.id,offset,revision.id,revision.state]);
 useEffect(()=>{const fresh=page.data?.items.find(r=>r.id===revision.id);if(fresh&&(fresh.state!==revision.state||fresh.title!==revision.title))onRevision(fresh)},[page.data,revision.id,revision.state,revision.title,onRevision]);
 const own=sameWorkspace(workspaceId,curriculum.workspaceId);const effectiveRole=own?role:'';
 return <section className="collaboration" aria-label="Collaborative authoring"><span className="eyebrow">Inspect / {curriculum.name}</span><h3>Collaborative authoring</h3>{!own&&<p role="status">Shared read-only curriculum. Comments, review, exports and editing are unavailable.</p>}
 <Notice {...page}/>{page.data&&<><div className="collab-actions"><label htmlFor="collab-revision">Revision</label><select id="collab-revision" value={revision.id} onChange={e=>{const next=page.data?.items.find(r=>r.id===e.target.value);if(next)onRevision(next)}}>{!page.data.items.some(r=>r.id===revision.id)&&<option value={revision.id}>{revision.title}</option>}{page.data.items.map(r=><option value={r.id} key={r.id}>{r.title} · {r.state}</option>)}</select><label htmlFor="collab-compare">Compare from</label><select id="collab-compare" value={from} onChange={e=>setFrom(e.target.value)}><option value="">Choose baseline</option>{page.data.items.map(r=><option value={r.id} key={r.id}>{r.title} · {r.state}</option>)}</select></div><Pager offset={offset} total={page.data.totalCount} onChange={n=>{setOffset(n);setFrom('')}} label="revisions"/></>}
 {from&&<RevisionDiff key={`${revision.id}:${from}`} revisionId={revision.id} from={from}/>}
 <RevisionContents key={revision.id} revision={revision} workspaceId={workspaceId} role={effectiveRole} refreshMetadata={page.reload}/>
 {own&&<Sharing curriculumId={curriculum.id} editable={canAuthor(role)}/>}
 </section>
}
function RevisionContents({revision,workspaceId,role,refreshMetadata}:{revision:Model<'PlanRevision'>;workspaceId:string;role:string;refreshMetadata:()=>void}){
 const [standardCode,setStandardCode]=useState('');
 const [version,setVersion]=useState(0);const [target,setTarget]=useState<CommentTarget>();const [title,setTitle]=useState('');const [kind,setKind]=useState<'unit'|'outcome'>('outcome');const [saving,setSaving]=useState(false);const [message,setMessage]=useState('');
 const result=useRemote(signal=>graph(revision.id,signal),[revision.id,version]);
 const editable=canAuthor(role)&&revision.state==='draft';const canComment=canAuthor(role)||role==='reviewer';
 function changed(){setVersion(v=>v+1);refreshMetadata()}
 async function create(){if(!title.trim()||(kind==='outcome'&&!standardCode))return;setSaving(true);setMessage('');try{await addNode(revision.id,kind,title,standardCode);setTitle('');changed();setMessage('Plan node created. Any old review must be renewed.')}catch(e){setMessage(problem(e))}finally{setSaving(false)}}
 return <><section className="collab-section"><h3>Plan nodes</h3><button className="secondary" onClick={changed}>Reload plan and review</button><Notice {...result}/>{result.data&&<ul className="collab-list">{result.data.nodes.map(node=><li key={node.id}><div><strong>{node.title}</strong><small>{node.kind}</small></div>{role&&<button className="secondary" onClick={()=>setTarget({kind:'node',id:node.id,title:node.title,revisionId:revision.id})}>Comments on {node.title}</button>}</li>)}</ul>}{result.data?.nodes.length===0&&<p>This draft has no nodes yet. Add an outcome or import a unit.</p>}
 {editable&&<form className="collab-form" onSubmit={e=>{e.preventDefault();void create()}}><label htmlFor="node-kind">New node kind</label><select id="node-kind" value={kind} onChange={e=>{setKind(e.target.value==='unit'?'unit':'outcome');setStandardCode('')}}><option value="outcome">Outcome</option><option value="unit">Unit</option></select><label htmlFor="node-title">Name</label><input id="node-title" required value={title} onChange={e=>setTitle(e.target.value)}/>{kind==='outcome'&&<OutcomeStandardPicker workspaceId={workspaceId} value={standardCode} onChange={setStandardCode} disabled={saving}/>}<button className="secondary" disabled={saving||!title.trim()||(kind==='outcome'&&!standardCode)}>Add node</button></form>}{message&&<p role="status">{message}</p>}</section>
 {role&&<><Review revisionId={revision.id} role={role} draft={revision.state==='draft'} version={version} displayedFingerprint={result.data?.contentFingerprint}/><MaterializedComments revisionId={revision.id} onInspect={setTarget}/>{result.data&&<UnitLibrary workspaceId={workspaceId} revisionId={revision.id} nodes={result.data.nodes} edges={result.data.edges} editable={editable} onChanged={changed}/>}</>}
 {target&&<CommentInspector key={`${target.kind}:${target.id}`} target={target} canComment={canComment} onClose={()=>setTarget(undefined)}/>}
 </>
}

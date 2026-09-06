import { useState } from 'react';
import { fromTemplate, policy, setPolicy, templates, problem } from '../api/collaboration';
import type { Model } from '../api/collaboration';
import { canAuthor, Notice, Pager, useRemote } from './common';

export function TemplateCreate({workspaceId,role,onCreated}:{workspaceId:string;role:string;onCreated:(c:Model<'Curriculum'>)=>void}){
 const [offset,setOffset]=useState(0);const [name,setName]=useState('');const [code,setCode]=useState('');const [saving,setSaving]=useState(false);const [message,setMessage]=useState('');
 const result=useRemote(signal=>templates(workspaceId,offset,signal),[workspaceId,offset]);
 async function submit(){setSaving(true);setMessage('');try{const cur=await fromTemplate(workspaceId,name,code);setName('');onCreated(cur);setMessage('Template curriculum and draft created.')}catch(e){setMessage(problem(e))}finally{setSaving(false)}}
 return <section className="collab-section"><h3>Create from a template</h3><button className="secondary" onClick={result.reload}>Reload templates</button><Notice {...result}/>{result.data&&<><form className="collab-form" onSubmit={e=>{e.preventDefault();void submit()}}><label htmlFor="template-choice">Template</label><select id="template-choice" value={code} onChange={e=>setCode(e.target.value)}><option value="">Choose a named template</option>{result.data.items.map(t=><option key={t.code} value={t.code}>{t.name}</option>)}</select><label htmlFor="template-name">Curriculum name</label><input id="template-name" required value={name} onChange={e=>setName(e.target.value)}/><button className="primary" disabled={!canAuthor(role)||!code||saving}>{saving?'Creating…':'Create from template'}</button></form><Pager offset={offset} total={result.data.totalCount} onChange={n=>{setOffset(n);setCode('')}} label="templates"/></>}{message&&<p role="status">{message}</p>}</section>
}
export function WorkspacePolicy({workspaceId,role}:{workspaceId:string;role:string}){
 const result=useRemote(signal=>policy(workspaceId,signal),[workspaceId]);
 return <section className="collab-section"><h3>Workspace collaboration policy</h3><Notice {...result}/>{result.data&&<PolicyForm key={JSON.stringify(result.data)} workspaceId={workspaceId} initial={result.data} editable={role==='owner'||role==='admin'} reload={result.reload}/>}</section>
}
function PolicyForm({workspaceId,initial,editable,reload}:{workspaceId:string;initial:Model<'CollaborationPolicy'>;editable:boolean;reload:()=>void}){
 const [p,setP]=useState(initial);const [message,setMessage]=useState('');const [saving,setSaving]=useState(false);
 async function submit(){if(!window.confirm(p.sharingEnabled?'Apply this collaboration policy?':'Disable sharing and revoke every outgoing workspace grant?'))return;setSaving(true);setMessage('');try{await setPolicy(workspaceId,p);setMessage('Policy saved.')}catch(e){setMessage(problem(e))}finally{setSaving(false)}}
 return <form className="collab-form" onSubmit={e=>{e.preventDefault();void submit()}}><label><input id="require-approval" name="requireApprovalForPublish" type="checkbox" disabled={!editable||saving} checked={p.requireApprovalForPublish} onChange={e=>setP({...p,requireApprovalForPublish:e.target.checked})}/> Require current reviewer approval to publish</label><label><input id="sharing-enabled" name="sharingEnabled" type="checkbox" disabled={!editable||saving} checked={p.sharingEnabled} onChange={e=>setP({...p,sharingEnabled:e.target.checked})}/> Enable read-only sharing</label><p>Disabling sharing revokes existing grants. Re-enabling does not restore them.</p><div className="collab-actions"><button className="primary" disabled={!editable||saving}>Save policy</button><button className="secondary" type="button" onClick={reload} disabled={saving}>Reload policy</button></div>{!editable&&<p>Only a local workspace owner or admin may change policy.</p>}{message&&<p role="status">{message}</p>}</form>
}

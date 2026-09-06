import { useEffect, useRef, useState } from 'react';
import { comment, comments, problem } from '../api/collaboration';
import type { CommentTarget } from '../api/collaboration';
import { Notice, Pager, useRemote } from './common';

export default function CommentInspector({target,canComment,onClose}:{target:CommentTarget;canComment:boolean;onClose:()=>void}){
 const dialog=useRef<HTMLDialogElement>(null);const [offset,setOffset]=useState(0);const [body,setBody]=useState('');const [saving,setSaving]=useState(false);const [message,setMessage]=useState('');
 const result=useRemote(signal=>comments(target,offset,signal),[target.kind,target.id,offset]);
 useEffect(()=>{const el=dialog.current;const opener=document.activeElement;el?.showModal();return()=>{el?.close();if(opener instanceof HTMLElement&&opener.isConnected)opener.focus()}},[]);
 const length=Array.from(body).length;
 async function submit(){setSaving(true);setMessage('');try{await comment(target,body);setBody('');setMessage('Comment saved.');result.reload()}catch(e){setMessage(problem(e))}finally{setSaving(false)}}
 return <dialog ref={dialog} className="comment-inspector" onCancel={e=>{e.preventDefault();if(!saving)onClose()}} aria-labelledby="comment-title">
 <div className="collab-actions"><span className="eyebrow">Inspect / {target.kind==='node'?'Plan node':'Materialized item'}</span><button className="secondary" autoFocus disabled={saving} onClick={onClose}>Close comments</button></div>
 <h3 id="comment-title">{target.title}</h3><Notice {...result}/>
 <button className="plain-button" disabled={saving} onClick={result.reload}>Reload comments</button>
 {result.data&&<><ol className="comment-thread">{result.data.items.map(c=><li key={c.id}><strong>{c.authorName||'Workspace member'}</strong><time dateTime={c.createdAt}>{new Date(c.createdAt).toLocaleString()}</time><p>{c.body}</p></li>)}</ol>{result.data.totalCount===0&&<p>No comments yet.</p>}<Pager offset={offset} total={result.data.totalCount} onChange={setOffset} label="comments"/></>}
 {canComment?<form className="collab-form" onSubmit={e=>{e.preventDefault();void submit()}}><label htmlFor="comment-body">Comment</label><textarea id="comment-body" value={body} onChange={e=>setBody(e.target.value)} disabled={saving} required aria-describedby="comment-length"/><small id="comment-length">{length} / 10,000 characters</small><button className="primary" disabled={saving||!body.trim()||length>10000}>{saving?'Saving…':'Post comment'}</button></form>:<p>Read-only membership: commenting is unavailable.</p>}
 {message&&<p role="status">{message}</p>}
 </dialog>
}

import { useState } from 'react';
import { runs, items } from '../api/collaboration';
import type { CommentTarget } from '../api/collaboration';
import { Notice, Pager, useRemote } from './common';
export default function MaterializedComments({revisionId,onInspect}:{revisionId:string;onInspect:(t:CommentTarget)=>void}){
 const [offset,setOffset]=useState(0);const [run,setRun]=useState('');
 const result=useRemote(signal=>runs(revisionId,offset,signal),[revisionId,offset]);
 return <section className="collab-section"><h3>Materialized items</h3><Notice {...result}/>{result.data&&<><label htmlFor="comment-run">Materialization run</label><select id="comment-run" value={run} onChange={e=>setRun(e.target.value)}><option value="">Choose a run</option>{result.data.items.map((r,i)=><option key={r.id} value={r.id}>Run {offset+i+1} · {r.status} · {new Date(r.createdAt).toLocaleString()}</option>)}</select>{!result.data.totalCount&&<p>No materializations yet.</p>}<Pager offset={offset} total={result.data.totalCount} onChange={n=>{setOffset(n);setRun('')}} label="runs"/></>}{run&&<ItemPage key={run} run={run} revisionId={revisionId} onInspect={onInspect}/>}</section>
}
function ItemPage({run,revisionId,onInspect}:{run:string;revisionId:string;onInspect:(t:CommentTarget)=>void}){
 const [offset,setOffset]=useState(0);const result=useRemote(signal=>items(run,offset,signal),[run,offset]);
 return <><Notice {...result}/>{result.data&&<><ul className="collab-list">{result.data.items.map(item=><li key={item.id}><strong>{item.title}</strong><button className="secondary" onClick={()=>onInspect({kind:'item',id:item.id,title:item.title,revisionId})}>Comment on {item.title}</button></li>)}</ul>{!result.data.totalCount&&<p>No items in this run.</p>}<Pager offset={offset} total={result.data.totalCount} onChange={setOffset} label="items"/></>}</>
}

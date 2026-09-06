import { useEffect, useState } from 'react';
import type { DependencyList } from 'react';
import { problem } from '../api/collaboration';

// Each caller declares its resource key in deps. Abort and discard old responses
// on selection/page changes; never display a previous workspace's result.
export function useRemote<T>(load:(signal:AbortSignal)=>Promise<T>,deps:DependencyList){
 const [version,setVersion]=useState(0);const key=JSON.stringify([...deps,version]);
 const [state,setState]=useState<{key:string;data?:T;error:string;loading:boolean}>({key:'',error:'',loading:true});
 useEffect(()=>{const controller=new AbortController();setState({key,error:'',loading:true});
 load(controller.signal).then(data=>{if(!controller.signal.aborted)setState({key,data,error:'',loading:false})}).catch(e=>{if(!controller.signal.aborted)setState({key,error:problem(e),loading:false})});
 return()=>controller.abort();
 // Resource keys are supplied by the caller, not the per-render loader closure.
 },[key]);
 const current=state.key===key?state:{data:undefined,error:'',loading:true};
 return {...current,reload:()=>setVersion(v=>v+1)};
}
export function Notice({loading,error}:{loading:boolean;error:string}){return <>{loading&&<p role="status">Loading…</p>}{error&&<p className="collab-error" role="alert">{error}</p>}</>}
export function Pager({offset,total,onChange,label='results'}:{offset:number;total:number;onChange:(n:number)=>void;label?:string}){return <nav className="collab-actions" aria-label={`${label} pages`}><button type="button" className="secondary" disabled={offset===0} onClick={()=>onChange(Math.max(0,offset-10))}>Previous {label}</button><span>{Math.min(offset+1,total)}–{Math.min(offset+10,total)} of {total}</span><button type="button" className="secondary" disabled={offset+10>=total} onClick={()=>onChange(offset+10)}>Next {label}</button></nav>}
export function canAuthor(role:string){return ['owner','admin','author'].includes(role)}
export function sameWorkspace(a:string,b:string){const norm=(v:string)=>v.replace(/^ws_/,'').replaceAll('-','');return norm(a)===norm(b)}

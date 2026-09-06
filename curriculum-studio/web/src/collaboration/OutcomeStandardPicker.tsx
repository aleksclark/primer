import { useState } from 'react';
import { catalogStandards, visibleCatalogs } from '../api/collaboration';
import { Notice, Pager, useRemote } from './common';

// Select real, workspace-visible catalog data. Never create a catalog or guess
// a standard merely to make publication pass. Both lists are server paginated.
export default function OutcomeStandardPicker({workspaceId,value,onChange,disabled}:{workspaceId:string;value:string;onChange:(code:string)=>void;disabled:boolean}) {
 const [offset,setOffset]=useState(0);const [catalog,setCatalog]=useState('');
 const result=useRemote(signal=>visibleCatalogs(workspaceId,offset,signal),[workspaceId,offset]);
 return <fieldset className="outcome-standards" disabled={disabled}>
  <legend>Outcome standard mapping</legend>
  <p>Choose an existing standard. Every outcome needs a real mapping before publication.</p>
  <Notice {...result}/>
  <button type="button" className="secondary" onClick={()=>{setCatalog('');onChange('');result.reload()}}>Reload catalogs</button>
  {result.data&&<><label htmlFor="outcome-catalog">Standards catalog</label>
   <select id="outcome-catalog" name="outcomeCatalog" value={catalog} onChange={e=>{setCatalog(e.target.value);onChange('')}}>
    <option value="">Choose a catalog</option>{(result.data.items??[]).map(c=><option key={c.id} value={c.id}>{c.title}</option>)}
   </select>
   {!result.data.totalCount&&<p>No visible catalogs. Import or curate standards before adding a publishable outcome.</p>}
   <Pager offset={offset} total={result.data.totalCount} onChange={n=>{setOffset(n);setCatalog('');onChange('')}} label="catalogs"/>
  </>}
  {catalog&&<StandardPage key={catalog} catalog={catalog} value={value} onChange={onChange}/>}
 </fieldset>;
}
function StandardPage({catalog,value,onChange}:{catalog:string;value:string;onChange:(code:string)=>void}) {
 const [offset,setOffset]=useState(0);const [draftQuery,setDraftQuery]=useState('');const [q,setQ]=useState('');
 const result=useRemote(signal=>catalogStandards(catalog,offset,q,signal),[catalog,offset,q]);
 function search(){setQ(draftQuery.trim());setOffset(0);onChange('');result.reload()}
 return <><label htmlFor="outcome-standard-search">Find an existing standard</label>
  <input id="outcome-standard-search" name="outcomeStandardSearch" value={draftQuery} onChange={e=>setDraftQuery(e.target.value)} onKeyDown={e=>{if(e.key==='Enter'){e.preventDefault();search()}}}/>
  <button type="button" className="secondary" onClick={search}>Search standards</button>
  <Notice {...result}/>
  {result.data&&<><label htmlFor="outcome-standard">Mapped standard</label>
   <select id="outcome-standard" name="outcomeStandard" required value={value} onChange={e=>onChange(e.target.value)}>
    <option value="">Choose a real standard</option>{(result.data.items??[]).map(s=><option key={s.id} value={s.code}>{s.code} — {s.description}</option>)}
   </select>
   {!result.data.totalCount&&<p>No matching standards. Change the search or catalog; no standard will be invented.</p>}
   <Pager offset={offset} total={result.data.totalCount} onChange={n=>{setOffset(n);onChange('')}} label="standards"/>
  </>}
 </>;
}

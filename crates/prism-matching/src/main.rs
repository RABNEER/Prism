use serde::{Deserialize, Serialize};
use std::collections::BTreeSet;
use std::io::{self, BufRead, Write};
use unicode_normalization::UnicodeNormalization;

const VERSION: &str = "0.3.0";

#[derive(Default, Deserialize)]
struct Record {
    #[serde(default)] id: String,
    #[serde(default)] state: String,
    #[serde(default)] district: String,
    #[serde(default)] locality: String,
    #[serde(default)] property_type: String,
    #[serde(default)] address: String,
    #[serde(default)] project: String,
    #[serde(default)] building: String,
    #[serde(default)] unit_id: String,
    #[serde(default)] provider_record_id: String,
    #[serde(default)] source_id: String,
    #[serde(default)] category: String,
    #[serde(default)] event_date: String,
    #[serde(default)] price_type: String,
    #[serde(default)] price: Option<f64>,
}

#[derive(Deserialize)] struct Pair { left: Record, right: Record }
#[derive(Deserialize)] struct Request { protocol_version: String, pairs: Vec<Pair> }
#[derive(Serialize)] struct Decision {
    left_id: String, right_id: String, kind: String,
    similarity: f64, reasons: Vec<String>, conflicts: Vec<String>,
}
#[derive(Serialize)] struct Response { protocol_version: String, worker_version: String, decisions: Vec<Decision> }

fn normalize(s: &str) -> String {
    // Preserve non-ASCII combining marks/scripts. Only ASCII punctuation is
    // split; aggressive transliteration can erase identity information.
    s.nfkc().flat_map(char::to_lowercase)
        .map(|c| if c.is_ascii_punctuation() { ' ' } else { c })
        .collect::<String>().split_whitespace()
        .map(|w| match w { "bldg" => "building", "apt" => "apartment", "rd" => "road", _ => w })
        .collect::<Vec<_>>().join(" ")
}

fn tokens(s: &str) -> BTreeSet<String> { normalize(s).split_whitespace().map(str::to_owned).collect() }

fn decide(a: Record, b: Record) -> Decision {
    let mut out = Decision { left_id:a.id.clone(), right_id:b.id.clone(), kind:"unresolved".into(), similarity:0.0, reasons:vec![], conflicts:vec![] };
    for (label,x,y) in [("state",&a.state,&b.state),("property_type",&a.property_type,&b.property_type),("district",&a.district,&b.district)] {
        if !x.is_empty() && !y.is_empty() && normalize(x)!=normalize(y) {out.kind="different".into();out.conflicts.push(format!("{label}_mismatch"));return out;}
    }
    let ta=tokens(&a.address);let tb=tokens(&b.address);
    let union=ta.union(&tb).count();
    out.similarity=if union==0 {0.0} else {ta.intersection(&tb).count() as f64/union as f64};
    let same_project=!a.project.is_empty() && normalize(&a.project)==normalize(&b.project);
    let same_building=!a.building.is_empty() && normalize(&a.building)==normalize(&b.building);
    let same_locality=!a.locality.is_empty() && normalize(&a.locality)==normalize(&b.locality);
    if !a.unit_id.is_empty() && !b.unit_id.is_empty() && normalize(&a.unit_id)!=normalize(&b.unit_id) {
        out.kind="different".into();out.conflicts.push("explicit_unit_mismatch".into());return out;
    }
    if a.price_type!=b.price_type {out.conflicts.push("price_type_differs".into());}
    let same_source=!a.source_id.is_empty() && a.source_id==b.source_id && !a.provider_record_id.is_empty() && a.provider_record_id==b.provider_record_id;
    if same_source {
        if a.category==b.category && a.event_date==b.event_date && a.price_type==b.price_type && a.price==b.price && normalize(&a.address)==normalize(&b.address) && normalize(&a.project)==normalize(&b.project) && normalize(&a.building)==normalize(&b.building) && normalize(&a.unit_id)==normalize(&b.unit_id) {
            out.kind="same_source_observation".into();out.reasons.push("same_source_record_and_matching_core_fields".into());
        } else {out.kind="source_revision_or_conflict".into();out.reasons.push("same_source_record_with_different_fields".into());}
        return out;
    }
    if same_locality && (same_project||same_building) && !a.unit_id.is_empty() && normalize(&a.unit_id)==normalize(&b.unit_id) {
        out.kind="same_unit_candidate".into();out.reasons.push("shared_project_or_building_and_explicit_unit_in_same_locality".into());
        if a.category=="transaction" && b.category=="transaction" && a.event_date!=b.event_date {out.reasons.push("different_transaction_dates_preserve_as_separate_sales".into());}
    } else if same_locality && (same_project||same_building) {
        out.kind="same_project_candidate".into();out.reasons.push("shared_project_or_building_but_unit_identity_unresolved".into());
    } else if out.similarity>=0.75 && same_locality {
        out.kind="address_candidate".into();out.reasons.push("address_token_overlap_only_requires_review".into());
    } else {out.reasons.push("insufficient_identity_evidence".into());}
    out
}

fn process(line: &str) -> Result<Response,String> {
    let req:Request=serde_json::from_str(line).map_err(|e|e.to_string())?;
    if req.protocol_version!="1" {return Err("unsupported protocol_version".into());}
    if req.pairs.len()>20000 {return Err("at most 20000 comparison pairs".into());}
    Ok(Response{protocol_version:"1".into(),worker_version:VERSION.into(),decisions:req.pairs.into_iter().map(|p|decide(p.left,p.right)).collect()})
}

fn main() {
    let mut out=io::BufWriter::new(io::stdout());
    for line in io::stdin().lock().lines() {
        let value=match line {Ok(line) if line.len()<=16*1024*1024=>match process(&line){Ok(r)=>serde_json::to_value(r).unwrap(),Err(e)=>serde_json::json!({"error":e})},_=>serde_json::json!({"error":"invalid or oversized request"})};
        if serde_json::to_writer(&mut out,&value).is_err() {break;}
        if out.write_all(b"\n").and_then(|_|out.flush()).is_err(){break;}
    }
}

#[cfg(test)] mod tests {
    use super::*;
    fn unit(id:&str,date:&str)->Record {Record{state:"Maharashtra".into(),district:"Pune".into(),locality:"Baner".into(),property_type:"apartment".into(),project:"Example".into(),unit_id:id.into(),category:"transaction".into(),event_date:date.into(),..Default::default()}}
    #[test] fn different_units_do_not_merge(){assert_eq!(decide(unit("101","2025-01-01"),unit("102","2025-01-01")).kind,"different");}
    #[test] fn repeated_sales_stay_separate(){let d=decide(unit("101","2024-01-01"),unit("101","2025-01-01"));assert_eq!(d.kind,"same_unit_candidate");assert!(d.reasons.iter().any(|r|r.contains("separate_sales")));}
    #[test] fn original_scripts_are_preserved(){assert_eq!(normalize("पुणे"),"पुणे");assert_ne!(normalize("पुणे"),normalize("पणे"));}
    #[test] fn same_source_revisions_are_not_duplicates(){let mut a=unit("101","2025-01-01");a.source_id="x".into();a.provider_record_id="a".into();let mut b=unit("101","2025-01-01");b.source_id="x".into();b.provider_record_id="a".into();b.price=Some(100.0);assert_eq!(decide(a,b).kind,"source_revision_or_conflict");}
}

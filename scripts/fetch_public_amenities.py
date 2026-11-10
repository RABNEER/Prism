"""Retrieve a bounded public Overture place extract as a source snapshot.

Invoked by PRISM's Go adapter. It never scans a complete country/release when
the spatial index is unavailable. stdout is a single machine-readable object.
"""
import argparse
import datetime
import math
import pathlib
import sys
import warnings

sys.stdout.reconfigure(encoding="utf-8")


def category(value):
    value = (value or "").lower()
    if any(t in value for t in ("school", "college", "university", "education")):
        return "education"
    if any(t in value for t in ("hospital", "medical_center", "health_center", "clinic")):
        return "healthcare"
    if "rail" in value or "subway" in value or "metro_station" in value:
        return "rail"
    if "bus_station" in value or "bus_stop" in value:
        return "bus"
    return None


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--latitude", type=float, required=True)
    p.add_argument("--longitude", type=float, required=True)
    p.add_argument("--radius-m", type=int, required=True)
    p.add_argument("--modules", required=True)
    args = p.parse_args()
    if not (6 <= args.latitude <= 38 and 68 <= args.longitude <= 98 and 100 <= args.radius_m <= 2000):
        raise ValueError("bounded Indian-region query required")
    sys.path.insert(0, str(pathlib.Path(args.modules).resolve()))
    from overturemaps.core import get_latest_release, record_batch_reader, StacIndexWarning
    from shapely import from_wkb
    warnings.simplefilter("error", StacIndexWarning)
    release = get_latest_release()
    dy = args.radius_m / 110000
    dx = args.radius_m / (110000 * math.cos(math.radians(args.latitude)))
    bbox = (args.longitude-dx, args.latitude-dy, args.longitude+dx, args.latitude+dy)
    reader = record_batch_reader("place", bbox=bbox, release=release, connect_timeout=10, request_timeout=30, stac=True)
    if reader is None:
        raise RuntimeError("source returned no reader")
    places = []
    scanned = 0
    for batch in reader:
        for row in batch.to_pylist():
            scanned += 1
            if scanned > 20000:
                raise RuntimeError("bounded extract exceeded 20000 records; narrow radius")
            addresses = row.get("addresses") or []
            # Country guard is explicit; missing country is not assumed India.
            if not any(a.get("country") == "IN" for a in addresses):
                continue
            taxonomy = row.get("taxonomy") or row.get("categories") or {}
            primary = taxonomy.get("primary") or row.get("basic_category") or ""
            cat = category(primary)
            if not cat:
                continue
            geometry = from_wkb(row["geometry"])
            if geometry.geom_type != "Point":
                continue
            places.append({"id": row["id"], "name": (row.get("names") or {}).get("primary", ""), "category": cat, "latitude": geometry.y, "longitude": geometry.x, "source_category": primary, "sources": row.get("sources") or []})
    import json
    print(json.dumps({"provider": "overture_places", "release": release, "retrieved_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "bbox": list(bbox), "scanned_records": scanned, "places": places, "attribution": "Overture Maps Foundation; upstream sources preserved per feature. Places source licenses include CDLA Permissive 2.0, Apache 2.0 and CC0; see https://docs.overturemaps.org/attribution/ and https://opensource.foursquare.com/places/NOTICE.txt"}, ensure_ascii=False, default=str))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        import json
        print(json.dumps({"error": str(error)}))
        raise SystemExit(1)

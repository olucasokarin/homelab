import urllib.request
import json
import urllib.parse
import os

RADARR_IP = os.environ.get("RADARR_IP", "192.168.1.110")
RADARR_PORT = os.environ.get("RADARR_PORT", "7878")
RADARR_API_KEY = os.environ.get("RADARR_API_KEY", "")

SONARR_IP = os.environ.get("SONARR_IP", "192.168.1.110")
SONARR_PORT = os.environ.get("SONARR_PORT", "8989")
SONARR_API_KEY = os.environ.get("SONARR_API_KEY", "")
def auto_import(app_name, ip, port, api_key, folder):
    print(f"Checking {app_name} for imports in {folder}...")
    
    # Get scan results
    params = {"folder": folder, "filterExistingFiles": "true"}
    query_string = urllib.parse.urlencode(params)
    url = f"http://{ip}:{port}/api/v3/manualimport?{query_string}"
    
    request = urllib.request.Request(url)
    request.add_header("X-Api-Key", api_key)
    
    try:
        with urllib.request.urlopen(request) as response:
            items = json.loads(response.read().decode())
    except Exception as e:
        print(f"  Error fetching: {e}")
        return

    import_items = []
    for item in items:
        # Check if movie/series is identified
        identified = False
        if app_name == "Radarr" and item.get("movie"):
            identified = True
        elif app_name == "Sonarr" and item.get("series"):
            identified = True
            
        if identified and not item.get("rejections"):
            movie_id = item.get("movie", {}).get("id") if app_name == "Radarr" else None
            series_id = item.get("series", {}).get("id") if app_name == "Sonarr" else None
            
            payload = {
                "path": item["path"],
                "importMode": "hardlink",
            }
            if movie_id: payload["movieId"] = movie_id
            if series_id: payload["seriesId"] = series_id
            if item.get("quality"): payload["quality"] = item["quality"]
            if item.get("releaseGroup"): payload["releaseGroup"] = item["releaseGroup"]
            if item.get("languages"): payload["language"] = item["languages"][0]["id"]
            if app_name == "Sonarr" and item.get("episodes"):
                payload["episodeIds"] = [e["id"] for e in item["episodes"]]
            
            import_items.append(payload)
            print(f"  Identified: {item['path']}")

    if import_items:
        print(f"  Importing {len(import_items)} items...")
        url = f"http://{ip}:{port}/api/v3/manualimport"
        request = urllib.request.Request(url, data=json.dumps(import_items).encode(), method='POST')
        request.add_header("X-Api-Key", api_key)
        request.add_header("Content-Type", "application/json")
        try:
            with urllib.request.urlopen(request) as response:
                print(f"  Import triggered: {response.status}")
        except Exception as e:
            print(f"  Error triggering import: {e}")
    else:
        print("  Nothing to import.")

if __name__ == "__main__":
    import sys
    if "/mnt/storage/downloads/radarr" in sys.argv or len(sys.argv) == 1:
        auto_import("Radarr", RADARR_IP, RADARR_PORT, RADARR_API_KEY, "/mnt/storage/downloads/radarr")
    if "/mnt/storage/downloads/sonarr" in sys.argv or len(sys.argv) == 1:
        auto_import("Sonarr", SONARR_IP, SONARR_PORT, SONARR_API_KEY, "/mnt/storage/downloads/sonarr")


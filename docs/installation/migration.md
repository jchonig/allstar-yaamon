# Migrating from AllScan or Allmon3

YAAMon has built-in import support — no conversion scripts needed. YAAMon can run alongside AllScan or Allmon3 during transition.

## From Allmon3

Go to **Admin → Nodes**, click **Import from Allmon3**, and upload your `allmon3.ini` file (usually `/etc/allmon3/allmon3.ini`). YAAMon parses the file and lists all nodes found. Nodes already in YAAMon are unchecked by default. Select the ones to import and confirm. AMI credentials are read directly from the file.

Imported nodes use the node number as the display name — rename them afterwards using the edit button.

## From AllScan

Go to **Favorites**, click **Import**, and upload your `favorites.ini` file (usually `/var/www/html/allscan/favorites.ini`). YAAMon parses the file, extracts node numbers and labels, and attempts to split each label into a callsign and description.

AllScan labels often follow the pattern `CALLSIGN Description` — YAAMon recognises a leading word as a callsign if it is 3–7 alphanumeric characters and contains at least one digit. A preview shows how many entries will be added and how many will be skipped (already exist). Confirm to import. Edit any favorite after import to correct the split if needed.

## Configuration

YAAMon uses its own SQLite database (`/var/lib/yaamon/yaamon.db`) and its own `config.yaml` — it does not share configuration files with AllScan or Allmon3. Both applications can run simultaneously on different ports during your evaluation period.

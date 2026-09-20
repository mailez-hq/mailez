# qa-tp-p13c-update-secret.ps1 - does PUT /fetches/:id seal the new password?
# Create encrypts the credential (P13.1b); the poller then reported
# "ciphertext too short" after a password update, which points at the update
# path storing the plaintext.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P13.1b update path rnd=$rnd =="
$admin = New-Session
$PW = "QaUser2026!"
$DST = "qa-p13c-dst-$rnd@example.com"
$SRC = "qa-p13c-src-$rnd@example.com"
foreach ($u in @($DST, $SRC)) { [void](Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P13c" }) }
$create = Api $admin POST "/fetches" @{ user_email = $DST; protocol = "pop3"; host = "engine-lb"; port = 995; tls = $true; keep = $true; username = $SRC; password = "CreateSecret-$rnd" }
$fid = if ($create.Data -and $create.Data.id) { [int]$create.Data.id } else { 0 }
$afterCreate = ""
try { $afterCreate = (docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT password FROM ``fetch`` WHERE id=$fid;" 2>$null | Select-Object -First 1) } catch {}

$update = Api $admin PUT "/fetches/$fid" @{ password = "UpdateSecret-$rnd" }
$afterUpdate = ""
try { $afterUpdate = (docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT password FROM ``fetch`` WHERE id=$fid;" 2>$null | Select-Object -First 1) } catch {}
$afterCreate = "$afterCreate".Trim(); $afterUpdate = "$afterUpdate".Trim()

Check P13.1c "PUT /fetches/:id seals the replaced credential too" `
    ($afterUpdate -ne "UpdateSecret-$rnd" -and $afterUpdate -ne "" -and $afterUpdate -ne $afterCreate) `
    "createStatus=$($create.Status) createStored=[$(if($afterCreate.Length -gt 14){$afterCreate.Substring(0,14)+'...'}else{$afterCreate})] updateStatus=$($update.Status) updateStored=[$afterUpdate] plaintextAfterUpdate=$($afterUpdate -eq "UpdateSecret-$rnd")"

# the update must also keep the poller working (no decrypt failure)
Start-Sleep -Seconds 3
$row = Api $admin GET "/fetches?user=$DST&limit=50"
$errRow = ""
foreach ($x in @($row.Data.data)) { if ([int]$x.id -eq $fid) { $errRow = "$($x.error)" } }
"row error after update: [$errRow]"

[void](Api $admin DELETE "/fetches/$fid")
[void](Api $admin DELETE "/users/$DST")
[void](Api $admin DELETE "/users/$SRC")
"cleanup done"
Flush-Results "P13"

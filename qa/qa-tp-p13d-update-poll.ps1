# qa-tp-p13d-update-poll.ps1 - after a credential rotation, what does the next
# poll report? Distinguishes "the update stored something the poller cannot
# decrypt" (ciphertext too short) from the expected "wrong password" rejection.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== P13 credential rotation poll probe rnd=$rnd =="
$admin = New-Session
$PW = "QaUser2026!"
$DST = "qa-p13d-dst-$rnd@example.com"
$SRC = "qa-p13d-src-$rnd@example.com"
foreach ($u in @($DST, $SRC)) { [void](Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA P13d" }) }
$create = Api $admin POST "/fetches" @{ user_email = $DST; protocol = "pop3"; host = "engine-lb"; port = 995; tls = $true; keep = $true; username = $SRC; password = $PW }
$fid = if ($create.Data -and $create.Data.id) { [int]$create.Data.id } else { 0 }

$put = Api $admin PUT "/fetches/$fid" @{ password = "WrongSecret-$rnd" }
$stored = ""
try { $stored = "$((docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e "SELECT password FROM ``fetch`` WHERE id=$fid;" 2>$null | Select-Object -First 1))".Trim() } catch {}

$err = ""
$sw = [Diagnostics.Stopwatch]::StartNew()
while ($sw.Elapsed.TotalSeconds -lt 400) {
    Start-Sleep -Seconds 20
    $r = Api $admin GET "/fetches?user=$DST&limit=50"
    foreach ($x in @($r.Data.data)) { if ([int]$x.id -eq $fid -and "$($x.error)".Trim() -ne "") { $err = "$($x.error)" } }
    if ($err -ne "") { break }
}
$kind = if ($err -match "ciphertext") { "decrypt failed (stored secret unreadable)" }
        elseif ($err -match "authentication failed|invalid credentials|ERR") { "expected credential rejection" }
        else { "no error surfaced" }
Check P13.1d "rotated credential reaches the poller as a credential, not a decrypt error" `
    ($put.Status -eq 200 -and $err -ne "" -and $err -notmatch "ciphertext") `
    "put=$($put.Status) storedPrefix=[$(if($stored.Length -gt 12){$stored.Substring(0,12)+'...'}else{$stored})] pollerError=[$err] class=$kind waited=$([int]$sw.Elapsed.TotalSeconds)s"

[void](Api $admin DELETE "/fetches/$fid")
[void](Api $admin DELETE "/users/$DST")
[void](Api $admin DELETE "/users/$SRC")
"cleanup done"
Flush-Results "P13"

# qa-cleanup-testdata.ps1 - remove the QA leftovers (TEST-PROMPTS 纪律 #3)
#
# Deletes every qa-* user and qa-* domain through the admin API so the
# engine-side cascade runs as well, then reports before/after counts for the
# tables the QA runs touch. Real domains (example.com) and the older
# distributed-test fixtures (e2e.example.com, test.dist) are left alone.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== QA test data cleanup =="

function MsgList($resp) {
    if ($null -eq $resp.Data) { return @() }
    if (@($resp.Data.PSObject.Properties.Name) -contains "data") { return @($resp.Data.data) }
    return @($resp.Data)
}
function Sql([string]$q) {
    try { return "$((docker exec mailez-mysql-1 mysql -umailez -pmailez mailez -N -e $q 2>$null | Select-Object -First 1))".Trim() } catch { return "" }
}

$admin = New-Session
"before: " + (Sql "SELECT CONCAT('users=',COUNT(*)) FROM ``user`` WHERE email LIKE 'qa-%';")
"before: " + (Sql "SELECT CONCAT('domains=',COUNT(*)) FROM domain WHERE name LIKE 'qa-%';")
"before: " + (Sql "SELECT CONCAT('aliases=',COUNT(*)) FROM alias WHERE email LIKE 'qa-%';")
"before: " + (Sql "SELECT CONCAT('fetches=',COUNT(*)) FROM ``fetch``;")

$users = MsgList (Api $admin GET "/users?page=1&limit=500")
$qa = @($users | Where-Object { $_.email -like "qa-*" })
"deleting $($qa.Count) qa-* users (of $($users.Count) total)"
$ok = 0; $fail = @()
foreach ($u in $qa) {
    $r = Api $admin DELETE "/users/$($u.email)"
    if ($r.Status -in @(200, 204)) { $ok++ } else { $fail += "$($u.email)=$($r.Status)" }
}
"users deleted: $ok, failed: $($fail.Count)$(if($fail.Count){' [' + (($fail | Select-Object -First 5) -join ',') + ']'}else{''})"

$domains = MsgList (Api $admin GET "/domains?page=1&limit=200")
$qaDomains = @($domains | Where-Object { $_.name -like "qa-*" })
$dok = 0; $dfail = @()
foreach ($d in $qaDomains) {
    $r = Api $admin DELETE "/domains/$($d.name)"
    if ($r.Status -in @(200, 204)) { $dok++ } else { $dfail += "$($d.name)=$($r.Status)" }
}
"domains deleted: $dok, failed: $($dfail.Count)$(if($dfail.Count){' [' + (($dfail | Select-Object -First 5) -join ',') + ']'}else{''})"

Start-Sleep -Seconds 8
"after: " + (Sql "SELECT CONCAT('users=',COUNT(*)) FROM ``user`` WHERE email LIKE 'qa-%';")
"after: " + (Sql "SELECT CONCAT('domains=',COUNT(*)) FROM domain WHERE name LIKE 'qa-%';")
"after: " + (Sql "SELECT CONCAT('aliases=',COUNT(*)) FROM alias WHERE email LIKE 'qa-%';")
"after: " + (Sql "SELECT CONCAT('fetches=',COUNT(*)) FROM ``fetch``;")
"remaining domains: " + (Sql "SELECT GROUP_CONCAT(name) FROM domain;")
"engine purge failures in the last 10m: " + (Sql "SELECT 'n/a';")
$purge = docker logs mailez-backend-1 --since 10m 2>&1 | Select-String -Pattern "engine purge" | Measure-Object | Select-Object -ExpandProperty Count
"backend 'engine purge ... failed' lines in last 10m: $purge"

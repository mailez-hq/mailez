# qa-fts-recheck.ps1 - is DEF-FTS-BODY still true? The 200-message load run
# found the body token through the API, so re-check the same message at the
# protocol level (IMAP SEARCH BODY/TEXT) and through the API.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$Base = if ($env:QA_BASE) { $env:QA_BASE } else { "http://localhost:8080/api/v1" }
"== FTS recheck rnd=$rnd =="
function MsgList($r){ if($null -eq $r.Data){return @()}; if(@($r.Data.PSObject.Properties.Name) -contains "data"){return @($r.Data.data)}; return @($r.Data) }

$admin = New-Session
$PW = "QaUser2026!"
$A = "qa-fts2-a-$rnd@example.com"
$B = "qa-fts2-b-$rnd@example.com"
foreach ($u in @($A, $B)) { [void](Api $admin POST "/users" @{ email = $u; password = $PW; display_name = "QA FTS" }) }
$aSess = New-Session $A $PW
$bSess = New-Session $B $PW
$token = "ftstoken$rnd"
$subj = "fts-subject-$rnd"
[void](Api $aSess POST "/mail/send" @{ to = @($B); subject = $subj; body = "unique body marker $token for full text search" })
Start-Sleep -Seconds 6

$api = Api $bSess GET "/mail/search?q=$token&folder=Inbox"
$apiHits = (MsgList $api).Count
$apiSubj = Api $bSess GET "/mail/search?q=$subj&folder=Inbox"
$apiSubjHits = (MsgList $apiSubj).Count
Check "FTS.1" "API body-keyword search finds the message" ($apiHits -ge 1) "hits=$apiHits status=$($api.Status); subjectKeywordHits=$apiSubjHits"

$py = @"
import imaplib, ssl, sys
ctx = ssl._create_unverified_context()
m = imaplib.IMAP4("127.0.0.1", 143)
m.starttls(ctx)
m.login("$B", "$PW")
m.select("INBOX")
def q(term):
    try:
        t, d = m.uid("search", None, term)
        return d[0].decode().strip() if d and d[0] else ""
    except Exception as e:
        return "ERR:" + str(e)[:40]
print("SEARCH BODY ->", q('BODY "$token"'))
print("SEARCH TEXT ->", q('TEXT "$token"'))
print("SEARCH SUBJECT ->", q('SUBJECT "$subj"'))
print("SEARCH ALL ->", q("ALL"))
m.logout()
"@
$out = $py | python -
$out | ForEach-Object { "  $_" }
$bodyHit = ($out -join " ") -match "SEARCH BODY -> \d"
$textHit = ($out -join " ") -match "SEARCH TEXT -> \d"
Check "FTS.2" "protocol-level BODY/TEXT search matches the message" ($bodyHit -and $textHit) "imap=[$($out -join ' | ')]"

[void](Api $admin DELETE "/users/$A")
[void](Api $admin DELETE "/users/$B")
"cleanup done"
Flush-Results "P6"

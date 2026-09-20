# scratch probe: what does POST /mail/send return for a plain two-user send?
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 999999
$env:QA_BASE = if ($env:QA_BASE) { $env:QA_BASE } else { "http://127.0.0.1:8081/api/v1" }
$admin = New-Session
$A = "qa-dbg-a-$rnd@example.com"
$B = "qa-dbg-b-$rnd@example.com"
foreach ($u in @($A, $B)) {
    $r = Api $admin POST "/users" @{ email = $u; password = "QaUser2026!"; display_name = "QA DBG" }
    "create $u -> $($r.Status)"
}
$b = New-Session $B "QaUser2026!"
"session B status=$($b.Status)"
$body = @{ to = @($A); subject = "dbg-$rnd"; body = "dbg body" }
"payload: $($body | ConvertTo-Json -Depth 8 -Compress)"
$s = Api $b POST "/mail/send" $body
"send status=$($s.Status) err=$($s.Err)"
if ($s.Raw) {
    $txt = [Text.Encoding]::UTF8.GetString($s.Raw.RawContentStream.ToArray())
    "resp body=$txt"
}
"== with text field instead =="
$s2 = Api $b POST "/mail/send" @{ to = @($A); subject = "dbg2-$rnd"; text = "dbg body" }
"send2 status=$($s2.Status)"
$ud1 = Api $admin DELETE "/users/$A"
$ud2 = Api $admin DELETE "/users/$B"
"cleanup: $($ud1.Status)/$($ud2.Status)"

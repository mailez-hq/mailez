# qa-batch-b1.ps1 — 邮件读路径 (B1..B5,B13,B18,B21)
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== B(1/2) 邮件读路径 rnd=$rnd =="
$s = New-Session
$qa = "qa-read-$rnd@example.com"
$nu = Api $s POST "/users" @{ email = $qa; password = "QaUser2026!"; display_name = "QA Read" }
$qs = New-Session -Email $qa -Pw "QaUser2026!"
"qa user: $qa create=$($nu.Status)"

# 准备：admin 给 qa 发 3 封（两封同主题构成会话 + 一封独立主题）
$tSubjA = "QA-b1-$rnd-thread alpha"
$tSubjR = "Re: $tSubjA"
$tSubjS = "QA-b1-$rnd-solo"
$m1 = Api $s POST "/mail/send" @{ to = @($qa); subject = $tSubjA; body = "thread root $rnd" }
$m2 = Api $s POST "/mail/send" @{ to = @($qa); subject = $tSubjR; body = "thread reply $rnd" }
$m3 = Api $s POST "/mail/send" @{ to = @($qa); subject = $tSubjS; body = "solo marker qab1solo$rnd" }
Start-Sleep -Seconds 6
"seed: m1=$($m1.Status) m2=$($m2.Status) m3=$($m3.Status)"

# B1 GET /mail/folders 含系统文件夹（大小写不敏感比对）
$f = Api $qs GET "/mail/folders"
$fnames = @()
if ($f.Data) { $fnames = @($f.Data | ForEach-Object { "$_" }) }
$hasInbox = ($fnames | Where-Object { $_ -match "^(?i)inbox$" }).Count -ge 1
$hasSent = ($fnames | Where-Object { $_ -match "(?i)sent" }).Count -ge 1
$hasDrafts = ($fnames | Where-Object { $_ -match "(?i)drafts" }).Count -ge 1
$hasTrash = ($fnames | Where-Object { $_ -match "(?i)trash" }).Count -ge 1
$hasJunk = ($fnames | Where-Object { $_ -match "(?i)junk" }).Count -ge 1
Check B1 "folders 列表含 Inbox/Sent/Drafts/Trash/Junk" ($f.Status -eq 200 -and $hasInbox -and $hasSent -and $hasDrafts -and $hasTrash -and $hasJunk) "status=$($f.Status) folders=$($fnames -join ',')"

# B2 GET /mail/messages 分页+排序（total 头与条数一致）
$qIn = Api $qs GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$totalHdr = ""
try { $totalHdr = [string]$qIn.Raw.Headers["X-Total-Messages"] } catch {}
$itemCount = @($qIn.Data).Count
$totalOk = $false
if ($totalHdr -ne "") { $totalOk = ([int]$totalHdr -ge $itemCount -and [int]$totalHdr -ge 3) }
$qSort = Api $qs GET "/mail/messages?folder=Inbox&page=0&sort=subject&dir=asc"
Check B2 "messages 分页+排序+total 一致" ($qIn.Status -eq 200 -and $totalOk -and $qSort.Status -eq 200) "list=$($qIn.Status) total=$totalHdr items=$itemCount sortSubj=$($qSort.Status)"

# B3 GET /mail/message 读信（含 subject/body/thread_id）
$msg = @($qIn.Data | Where-Object { $_.subject -eq $tSubjS }) | Select-Object -First 1
$detail = $null
if ($msg) { $detail = Api $qs GET "/mail/message?folder=Inbox&uid=$($msg.uid)" }
$dSubj = ""; $dBody = ""
if ($detail.Data) { $dSubj = $detail.Data.subject; $dBody = "$($detail.Data.text_body)$($detail.Data.html_body)" }
Check B3 "message 读信含 subject/body" ($detail -ne $null -and $detail.Status -eq 200 -and $dSubj -eq $tSubjS -and $dBody.Length -gt 0) "status=$($detail.Status) subj=$dSubj bodyLen=$($dBody.Length)"

# B4 GET /mail/raw 原始 RFC822
$raw = $null
if ($msg) { $raw = Api $qs GET "/mail/raw?folder=Inbox&uid=$($msg.uid)" }
$rawTxt = ""; $rawOk = $false
if ($raw -and $raw.Data -and $raw.Data.raw) {
    $rawTxt = $raw.Data.raw
    $rawOk = ($rawTxt -match "Subject:" -and $rawTxt -match [regex]::Escape($tSubjS))
}
Check B4 "raw 返回原始 RFC822" ($raw -ne $null -and $raw.Status -eq 200 -and $rawOk) "status=$($raw.Status) rawLen=$($rawTxt.Length)"

# B5 GET /mail/thread 会话聚合（优先取同主题两封那条会话，>= 2 条）
$thrMsg = @($qIn.Data | Where-Object { $_.subject -eq $tSubjA }) | Select-Object -First 1
$tid = ""
if ($thrMsg) {
    $tmp = Api $qs GET "/mail/message?folder=Inbox&uid=$($thrMsg.uid)"
    if ($tmp.Data) { $tid = $tmp.Data.thread_id }
}
if ($tid -eq "" -and $detail -and $detail.Data) { $tid = $detail.Data.thread_id }
$thr = $null; $thrN = 0
if ($tid -ne "") {
    $thr = Api $qs GET "/mail/thread?folder=Inbox&thread_id=$([uri]::EscapeDataString($tid))"
    if ($thr.Data -and $thr.Data.messages) { $thrN = @($thr.Data.messages).Count }
}
Check B5 "thread 会话聚合" ($thr -ne $null -and $thr.Status -eq 200 -and $thrN -ge 2) "status=$($thr.Status) tid=$tid msgs=$thrN"

# B13 GET /mail/search 关键字命中/未命中
$hitQ = Api $qs GET "/mail/search?folder=Inbox&q=$([uri]::EscapeDataString("qab1solo$rnd"))"
$hitN = @($hitQ.Data).Count
$missQ = Api $qs GET "/mail/search?folder=Inbox&q=$([uri]::EscapeDataString("no-such-word-$rnd"))"
$missN = @($missQ.Data).Count
Check B13 "search 关键字命中且无关词为空" ($hitQ.Status -eq 200 -and $hitN -ge 1 -and $missQ.Status -eq 200 -and $missN -eq 0) "hit=$($hitQ.Status)/$hitN miss=$($missQ.Status)/$missN"

# B18 GET /mail/unseen 与未读数一致（3 封新信未读）
$un = Api $qs GET "/mail/unseen"
$ib = $null
if ($un.Data) { $ib = $un.Data.Inbox; if ($null -eq $ib) { $ib = $un.Data.INBOX } }
Check B18 "unseen Inbox >= 3（3封未读）" ($un.Status -eq 200 -and $ib -ne $null -and [int]$ib -ge 3) "status=$($un.Status) unseen.Inbox=$ib"

# B21 GET /mail/identities 含本人地址
$ids = Api $qs GET "/mail/identities"
$idHit = $false
$idN = 0
if ($ids.Data) { $idN = @($ids.Data).Count; $idHit = (@($ids.Data | Where-Object { $_.email -eq $qa }).Count -ge 1) }
Check B21 "identities 含本人地址" ($ids.Status -eq 200 -and $idHit) "status=$($ids.Status) n=$idN hit=$idHit"

Flush-Results "B1"
"== done =="

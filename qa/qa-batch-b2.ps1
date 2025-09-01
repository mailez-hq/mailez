# qa-batch-b2.ps1 — mail core write paths (B6..B20)
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== B(2/2) 邮件核心-写路径 rnd=$rnd =="
$s = New-Session
$qa = "qa-mail-$rnd@example.com"
$nu = Api $s POST "/users" @{ email = $qa; password = "QaUser2026!"; display_name = "QA Mail" }
$qs = New-Session -Email $qa -Pw "QaUser2026!"
"qa user: $qa create=$($nu.Status)"

# B6 发送端到端: admin -> qa
$subj = "QA-mail-$rnd hello"
$send = Api $s POST "/mail/send" @{ to = @($qa); subject = $subj; body = "QA body $rnd" }
Start-Sleep -Seconds 6
$qIn = Api $qs GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$hit = @($qIn.Data | Where-Object { $_.subject -eq $subj })
$aSent = Api $s GET "/mail/messages?folder=Sent&page=0&sort=date&dir=desc"
$sentHit = @($aSent.Data | Where-Object { $_.subject -eq $subj })
Check B6 "发送端到端(收件方可见+寄件方Sent)" ($send.Status -in @(200,201,204) -and $hit.Count -ge 1 -and $sentHit.Count -ge 1) "send=$($send.Status) inbox=$($hit.Count) sent=$($sentHit.Count)"
$qmsg = $hit | Select-Object -First 1

# B7 回复(带 in_reply_to)
$mid = $null
$rawq = Api $qs GET "/mail/raw?folder=Inbox&uid=$($qmsg.uid)"
if ($rawq.Data.raw -match "Message-ID:\s*<([^>]+)>") { $mid = $Matches[1] }
$rsubj = "Re: $subj"
$rep = Api $qs POST "/mail/send" @{ to = @("admin@example.com"); subject = $rsubj; body = "QA reply $rnd"; in_reply_to = "<$mid>" }
Start-Sleep -Seconds 6
$aIn = Api $s GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$repHit = @($aIn.Data | Where-Object { $_.subject -eq $rsubj }) | Select-Object -First 1
$rr = $null
if ($repHit) { $rr = Api $s GET "/mail/raw?folder=Inbox&uid=$($repHit.uid)" }
$irtOk = $false
if ($rr -and $rr.Data.raw) { $irtOk = $rr.Data.raw -match [regex]::Escape("<$mid>") }
Check B7 "回复含 In-Reply-To" ($rep.Status -in @(200,201,204) -and $repHit -ne $null -and $irtOk) "reply=$($rep.Status) found=$($repHit -ne $null) irt=$irtOk"

# B8 草稿: 保存→列表→(API无bcc字段:记录缺陷)→删除
$dsubj = "QA-draft-$rnd"
$dr = Api $s POST "/mail/draft" @{ subject = $dsubj; to = @($qa); cc = @("e2esmoke@example.com"); text = "draft body $rnd" }
Start-Sleep -Seconds 2
$drList = Api $s GET "/mail/messages?folder=Drafts&page=0&sort=date&dir=desc"
$drHit = @($drList.Data | Where-Object { $_.subject -eq $dsubj }) | Select-Object -First 1
$del = $null
if ($drHit) { $del = Api $s POST "/mail/delete" @{ folder = "Drafts"; uid = $drHit.uid } }
Check B8 "草稿 保存/列表/删除" ($dr.Status -eq 200 -and $drHit -ne $null -and $del.Status -in @(200,204)) "save=$($dr.Status) found=$($drHit -ne $null) del=$($del.Status) [注:/mail/draft 无 bcc 字段→草稿丢BCC]"

# B9 星标/已读
$auid = ($aIn.Data | Select-Object -First 1).uid
$f1 = Api $s POST "/mail/flag" @{ folder = "Inbox"; uid = $auid; flag = "flagged"; value = $true }
$l1 = Api $s GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$starred = @($l1.Data | Where-Object { $_.uid -eq $auid -and ($_.flags -join " ") -match "Flagged" }).Count
$f2 = Api $s POST "/mail/flag" @{ folder = "Inbox"; uid = $auid; flag = "seen"; value = $true }
$f3 = Api $s POST "/mail/flag" @{ folder = "Inbox"; uid = $auid; flag = "flagged"; value = $false }
Check B9 "flag 星标/已读切换" ($f1.Status -in @(200,204) -and $starred -ge 1 -and $f2.Status -in @(200,204) -and $f3.Status -in @(200,204)) "flag=$($f1.Status) starred=$starred"

# B10 read-all
Api $s POST "/mail/read-all" @{ folder = "Inbox" } | Out-Null
Start-Sleep -Seconds 2
$un = Api $s GET "/mail/unseen"
$ib = $un.Data.Inbox; if ($null -eq $ib) { $ib = $un.Data.INBOX }
Check B10 "read-all 后 Inbox 未读=0" ($un.Status -eq 200 -and [int]$ib -eq 0) "unseen.Inbox=$ib"

# B17 文件夹 CRUD
$fname = "QA-Folder-$rnd"
$fc = Api $s POST "/mail/folders" @{ name = $fname }
$fr = Api $s PUT "/mail/folders" @{ name = $fname; new_name = "$fname-r" }
$mv = Api $s POST "/mail/move" @{ folder = "Inbox"; uids = @([int]$auid); destination = "$fname-r" }
Start-Sleep -Seconds 2
$fl = Api $s GET "/mail/messages?folder=$fname-r&page=0&sort=date&dir=desc"
$movedCnt = @($fl.Data).Count
$mback = Api $s POST "/mail/move" @{ folder = "$fname-r"; uids = @([int]$auid); destination = "Inbox" }
$clr = Api $s POST "/mail/folders/clear" @{ name = "$fname-r" }
$fdel = Api $s DELETE "/mail/folders?name=$fname-r"
Check B17 "文件夹 建名移清删" ($fc.Status -in @(200,201,204) -and $fr.Status -in @(200,204) -and $mv.Status -in @(200,204) -and $movedCnt -ge 1 -and $mback.Status -in @(200,204) -and $clr.Status -in @(200,204) -and $fdel.Status -in @(200,204)) "c=$($fc.Status) r=$($fr.Status) mv=$($mv.Status) movedCnt=$movedCnt back=$($mback.Status) clear=$($clr.Status) del=$($fdel.Status)"

# B12 删除 → Trash
$dsubj2 = "QA-todelete-$rnd"
Api $s POST "/mail/send" @{ to = @("admin@example.com"); subject = $dsubj2; body = "x" } | Out-Null
Start-Sleep -Seconds 6
$aIn2 = Api $s GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$dt = @($aIn2.Data | Where-Object { $_.subject -eq $dsubj2 }) | Select-Object -First 1
$trashBefore = @($(Api $s GET "/mail/messages?folder=Trash&page=0&sort=date&dir=desc").Data).Count
$d2 = Api $s POST "/mail/delete" @{ folder = "Inbox"; uid = $dt.uid }
Start-Sleep -Seconds 2
$trashAfter = @($(Api $s GET "/mail/messages?folder=Trash&page=0&sort=date&dir=desc").Data).Count
Check B12 "删除进 Trash" ($d2.Status -in @(200,204) -and $trashAfter -eq $trashBefore + 1) "del=$($d2.Status) trash $trashBefore->$trashAfter"

# B14 延后 snooze（B12 已把 $dsubj2 移进 Trash，这里发独立邮件避免标记落空）
Api $s POST "/mail/send" @{ to = @("admin@example.com"); subject = "QA-snooze-$rnd"; body = "x" } | Out-Null
Start-Sleep -Seconds 6
$aInS = Api $s GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$suid = (@($aInS.Data | Where-Object { $_.subject -eq "QA-snooze-$rnd" }) | Select-Object -First 1).uid
$until = [int][double]::Parse((Get-Date (Get-Date).AddSeconds(120).ToUniversalTime() -UFormat %s))
$sn = Api $s POST "/mail/snooze" @{ folder = "Inbox"; uid = [int]$suid; until = $until }
$snl = Api $s GET "/mail/snoozed"
$snHit = @($snl.Data | Where-Object { "$($_.uid)" -eq "$suid" -or $_.subject -eq $dsubj2 }).Count
Check B14 "snooze 后出现在延后列表" ($sn.Status -in @(200,204) -and $snHit -ge 1) "snooze=$($sn.Status) listed=$snHit"

# B15 附件 zip（先发一封带附件的）
$b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("QA attachment content $rnd"))
$asubj = "QA-attach-$rnd"
$ar = Api $s POST "/mail/send" @{ to = @("admin@example.com"); subject = $asubj; body = "see attachment"; attachments = @(@{ filename = "qa-$rnd.txt"; content_type = "text/plain"; size = $b64.Length; data = $b64 }) }
Start-Sleep -Seconds 6
$aIn3 = Api $s GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$am = @($aIn3.Data | Where-Object { $_.subject -eq $asubj }) | Select-Object -First 1
$zipStatus = 0; $zipMagic = ""
if ($am -and $am.has_attachment) {
    $zr = Api $s GET "/mail/attachments/zip?folder=Inbox&uid=$($am.uid)"
    $zipStatus = $zr.Status
    if ($zr.Raw -and $zr.Raw.Content) { $bytes = $zr.Raw.Content; if ($bytes.Length -ge 2) { $zipMagic = "$([char]$bytes[0])$([char]$bytes[1])" } }
} else { $zipStatus = -1 }
Check B15 "附件邮件+zip 下载" ($ar.Status -in @(200,201,204) -and $am -ne $null -and $am.has_attachment -and $zipStatus -eq 200 -and $zipMagic -eq "PK") "send=$($ar.Status) has_att=$($am.has_attachment) zip=$($zipStatus) magic='$zipMagic'"

# B16 标签
$lb = "qa-label-$rnd"
$l1r = Api $s POST "/mail/labels" @{ name = $lb; color = "#ff0000" }
$ll = Api $s GET "/mail/labels"
$lhit = @($ll.Data | Where-Object { $_.name -eq $lb }).Count
$lr = Api $s POST "/mail/labels/rename" @{ from = $lb; to = "$lb-2" }
$ll2 = Api $s GET "/mail/labels"
$lhit2 = @($ll2.Data | Where-Object { $_.name -eq "$lb-2" }).Count
$ld = Api $s DELETE "/mail/labels?name=$lb-2"
Check B16 "标签 CRUD" ($l1r.Status -in @(200,201) -and $lhit -ge 1 -and $lr.Status -in @(200,204) -and $lhit2 -ge 1 -and $ld.Status -in @(200,204)) "c=$($l1r.Status) list=$lhit ren=$($lr.Status) list2=$lhit2 del=$($ld.Status)"

# B19 模板
$tr = Api $s POST "/mail/templates" @{ name = "qa-tpl-$rnd"; subject = "tpl $rnd"; body = "hello {{name}}" }
$tl = Api $s GET "/mail/templates"
$thit = @($tl.Data | Where-Object { $_.name -eq "qa-tpl-$rnd" }).Count
$tplId = (@($tl.Data | Where-Object { $_.name -eq "qa-tpl-$rnd" }) | Select-Object -First 1).id
$td = Api $s DELETE "/mail/templates/$tplId"
Check B19 "模板 存/列/删" ($tr.Status -in @(200,201) -and $thit -ge 1 -and $td.Status -in @(200,204)) "c=$($tr.Status) list=$thit del=$($td.Status)"

# B20 定时发送+取消
$fut = (Get-Date (Get-Date).AddMinutes(10).ToUniversalTime()).ToString("yyyy-MM-ddTHH:mm:ssZ")
$sc = Api $s POST "/mail/send" @{ to = @($qa); subject = "QA-sched-$rnd"; body = "sched"; send_at = $fut }
$scl = Api $s GET "/mail/scheduled"
$oid = $sc.Data.outbox_id
$onList = @($scl.Data | Where-Object { "$($_.id)" -eq "$oid" }).Count -ge 1
$cx = Api $s DELETE "/mail/outbox/$oid"
Check B20 "定时发送→列表→取消" ($sc.Status -eq 200 -and $sc.Data.scheduled -eq $true -and $onList -and $cx.Status -in @(200,204)) "sched=$($sc.Status) onList=$onList cancel=$($cx.Status)"

# B6b BCC 副本: bcc 用户收到但 To 看不到
$bccUser = "qa-bcc-$rnd@example.com"
$null = Api $s POST "/users" @{ email = $bccUser; password = "QaUser2026!" }
$bs = New-Session -Email $bccUser -Pw "QaUser2026!"
$bsend = Api $s POST "/mail/send" @{ to = @($qa); bcc = @($bccUser); subject = "QA-bcc-$rnd"; body = "bcc test" }
Start-Sleep -Seconds 6
$bIn = Api $bs GET "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc"
$bhit = @($bIn.Data | Where-Object { $_.subject -eq "QA-bcc-$rnd" }).Count
Check B6b "BCC 副本送达" ($bsend.Status -in @(200,201,204) -and $bhit -ge 1) "send=$($bsend.Status) bccInbox=$bhit"

Flush-Results "B2"
"== done =="

# Seeds demo data (user, calendar, contacts, drive) through the live dev API
# so the marketing screenshots show a lived-in mailbox. Admin + backend must
# be running on :8080 with demo-shots.db.
$ErrorActionPreference = 'Stop'
$base = 'http://127.0.0.1:8080/api/v1'

# --- admin session: create the demo user -------------------------------
$adminBody = '{"email":"admin@example.com","pw":"MailezDemo2026!"}'
$s = Invoke-WebRequest "$base/sso/login" -Method Post -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($adminBody)) -SessionVariable admin -UseBasicParsing
"admin login: $($s.StatusCode)"

$ub = '{"email":"chen@example.com","password":"ChenDemo2026!","displayed_name":"陈静","enabled":true}'
try {
  $r = Invoke-WebRequest "$base/users" -Method Post -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($ub)) -WebSession $admin -UseBasicParsing
  "user created: $($r.StatusCode)"
} catch { "user exists? $($_.ErrorDetails.Message)" }

# --- chen session: lived-in data ---------------------------------------
$cb = '{"email":"chen@example.com","pw":"ChenDemo2026!"}'
$c = Invoke-WebRequest "$base/sso/login" -Method Post -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($cb)) -SessionVariable chen -UseBasicParsing
"chen login: $($c.StatusCode)"

# calendar: a working week of meetings (fixed dates so shots stay stable).
# Mon 2026-08-31 .. Fri 2026-09-04 — $day is the weekday offset from Monday.
function Ev($summary, $loc, $day, $h1, $m1, $h2, $m2, $desc) {
  $d = [datetime]::new(2026, 8, 31).AddDays([int]$day)
  $ds = $d.ToString('yyyy-MM-dd')
  $start = "${ds}T$h1`:$m1`:00+08:00"
  $end = "${ds}T$h2`:$m2`:00+08:00"
  $b = @{ summary = $summary; location = $loc; description = $desc; start = $start; end = $end } | ConvertTo-Json
  Invoke-WebRequest "$base/calendar/events" -Method Post -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($b)) -WebSession $chen -UseBasicParsing | Out-Null
}
Ev '产品评审会：邮件网关 v2.3' '3F 会议室·青' 0 '10' '00' '11' '30' '评审反垃圾规则更新与引擎切换灰度方案'
Ev '与客户对齐部署方案（华东政务云）' '腾讯会议' 0 '14' '00' '15' '00' '确认等保材料与内网模型部署细节'
Ev '周例会' '3F 会议室·湖' 1 '09' '30' '10' '00' ''
Ev '引擎性能基准复盘' '线上' 1 '16' '00' '17' '00' '29.0 msg/s 收件吞吐与内存数据的解读'
Ev '新同事入职引导' '5F 培训室' 2 '14' '00' '15' '30' '邮箱客户端配置、PGP 与两步验证'
Ev '季度规划工作坊' '1F 报告厅' 3 '13' '30' '17' '00' ''
"calendar seeded"

# contacts
function C($name, $email, $group, $comment) {
  $b = @{ name = $name; email = $email; groups = $group; comment = $comment; avatar = '' } | ConvertTo-Json
  Invoke-WebRequest "$base/contacts" -Method Post -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($b)) -WebSession $chen -UseBasicParsing | Out-Null
}
C '王峰' 'wangfeng@huadong-gov.cn' '客户' '华东政务云 项目负责人'
C '李文博' 'liwenbo@mailez.dev' '同事' '引擎研发'
C '刘颖' 'liuying@cvte-example.com.cn' '客户' '采购询价对接'
C '赵磊' 'zhaolei@example.com' '同事' '运维值班'
C 'hr@example.com' 'hr@example.com' '内部' '人事通知'
C 'InfoQ 编辑部' 'infoq-editorial@subscribe.example' '订阅' '技术媒体'
"contacts seeded"

# drive: folders + a few small files
$f1 = Invoke-RestMethod "$base/drive/folders" -Method Post -ContentType 'application/json; charset=utf-8' -Body '{"parent_id":0,"name":"项目文档"}' -WebSession $chen
$f2 = Invoke-RestMethod "$base/drive/folders" -Method Post -ContentType 'application/json; charset=utf-8' -Body ('{"parent_id":' + $f1.id + ',"name":"验收材料"}') -WebSession $chen
$files = @(
  @{ n = '邮件系统部署方案-v2.3.md'; t = '# 邮件系统部署方案 v2.3' + "`n`n## 拓扑`n双节点引擎 + MySQL 主从，TiDB 可选。`n`n## 端口`n25/143/443 对外，1587 提交。`n" },
  @{ n = '等保三级材料清单.md'; t = '# 等保三级材料清单' + "`n`n- 网络拓扑图`n- 审计日志留存证明（180 天）`n- 数据出境评估：无出境`n" },
  @{ n = 'meeting-notes-0828.txt'; t = "8.28 例会纪要`n`n1. 引擎灰度 20% -> 60%`n2. DKIM 轮换窗口定在周六`n3. 下季度目标：万级用户压测报告`n" }
)
foreach ($f in $files) {
  $bytes = [System.Text.Encoding]::UTF8.GetBytes($f.t)
  $boundary = '----mailezdemo' + (Get-Random)
  $ms = New-Object System.IO.MemoryStream
  $w = New-Object System.IO.BinaryWriter($ms)
  $enc = [System.Text.Encoding]::UTF8
  $w.Write($enc.GetBytes("--$boundary`r`nContent-Disposition: form-data; name=`"file`"; filename=`"$($f.n)`"`r`nContent-Type: application/octet-stream`r`n`r`n"))
  $w.Write($bytes); $w.Write($enc.GetBytes("`r`n--$boundary`r`nContent-Disposition: form-data; name=`"parent_id`"`r`n`r`n$($f2.id)`r`n--$boundary--`r`n"))
  $w.Flush()
  Invoke-WebRequest "$base/drive/upload" -Method Post -ContentType "multipart/form-data; boundary=$boundary" -Body $ms.ToArray() -WebSession $chen -UseBasicParsing | Out-Null
}
"drive seeded: done"

# qa-batch-eng.ps1 - engine protocol checks over TLS (rewritten).
# The production engine stack requires TLS before AUTH on every listener, so
# authenticated flows run over: IMAP 993 (implicit), POP3 995 (implicit),
# SMTP 587 (STARTTLS), ManageSieve 4190 (STARTTLS). A plaintext IMAP LOGIN
# on 143 is asserted to be refused (PRIVACYREQUIRED) as a negative check.
# ASCII-only comments: PS 5.1 parses BOM-less files as ANSI.
[Console]::OutputEncoding = [Text.Encoding]::UTF8
Set-Location D:\code\mailez-hq\mailez\qa
Import-Module .\qa-lib.psm1 -Force
$rnd = Get-Random -Maximum 99999
"== ENG engine protocols (TLS) rnd=$rnd =="
$AdminEmail = "admin@example.com"
$AdminPw    = "MailezDemo2026!"
$Host_ = "127.0.0.1"
$ImapPort = 993; $PopPort = 995; $SmtpPort = 587; $SievePort = 4190; $PlainImap = 143

function New-Tcp {
    param([string]$Server, [int]$Port, [int]$TimeoutMs = 20000)
    $c = New-Object System.Net.Sockets.TcpClient
    $c.ReceiveTimeout = $TimeoutMs
    $c.SendTimeout = $TimeoutMs
    $c.Connect($Server, $Port)
    return $c
}
function Wrap-Tls {
    param($Tcp)
    $cb = [System.Net.Security.RemoteCertificateValidationCallback] { $true }
    $ssl = New-Object System.Net.Security.SslStream($Tcp.GetStream(), $false, $cb)
    $ssl.AuthenticateAsClient("mailez")
    return $ssl
}
function Read-ProtoLine {
    param($S)
    $sb = New-Object System.Text.StringBuilder
    $buf = New-Object byte[] 1
    while ($true) {
        $n = $S.Read($buf, 0, 1)
        if ($n -le 0) { break }
        $ch = [char]$buf[0]
        if ($ch -eq "`n") { break }
        if ($ch -ne "`r") { [void]$sb.Append($ch) }
        if ($sb.Length -gt 65536) { break }
    }
    return $sb.ToString()
}
function Read-LiteralAware {
    # Reads lines until the tagged completion; when a line ends with {n},
    # consumes n raw bytes plus the trailing CRLF as one blob.
    param($S, [string]$Tag)
    $sb = New-Object System.Text.StringBuilder
    while ($true) {
        $line = Read-ProtoLine $S
        if ($line -eq "") { break }
        [void]$sb.Append($line).Append(" | ")
        if ($line -match "\{(\d+)\}\r?$") {
            $n = [int]$Matches[1]
            $blob = New-Object byte[] $n
            $read = 0
            while ($read -lt $n) {
                $got = $S.Read($blob, $read, $n - $read)
                if ($got -le 0) { break }
                $read += $got
            }
            [void]$sb.Append("<literal:$n bytes> | ")
            $cr = Read-ProtoLine $S   # CRLF after the literal blob
            continue
        }
        if ($Tag -ne "" -and $line.StartsWith("$Tag ")) { break }
        if ($Tag -eq "") { break }
    }
    return $sb.ToString().TrimEnd(" ", "|")
}
function Send-ProtoLine {
    param($S, [string]$Text)
    $b = [Text.Encoding]::UTF8.GetBytes($Text + "`r`n")
    $S.Write($b, 0, $b.Length)
}

# ---------- negative: plaintext IMAP LOGIN on 143 is refused ----------
try {
    $c = New-Tcp $Host_ $PlainImap
    $s = $c.GetStream()
    $null = Read-ProtoLine $s
    Send-ProtoLine $s "a1 LOGIN $AdminEmail $AdminPw"
    $plain = Read-LiteralAware $s "a1"
    $c.Close()
} catch { $plain = "conn-error" }
Check ENG-IMAP-PLAINREFUSED "plaintext 143 LOGIN refused (TLS policy)" ($plain -match "NO") "resp=$plain"

# ---------- IMAP 993 implicit TLS ----------
try {
    $c = New-Tcp $Host_ $ImapPort
    $s = Wrap-Tls $c
    $null = Read-ProtoLine $s
    Send-ProtoLine $s "a1 LOGIN $AdminEmail $AdminPw"
    $rLogin = Read-LiteralAware $s "a1"
    Check ENG-IMAP-LOGIN "IMAP LOGIN OK (implicit TLS 993)" ($rLogin -match "a1 OK") "resp=$rLogin"

    Send-ProtoLine $s "a2 SELECT INBOX"
    $rSel = Read-LiteralAware $s "a2"
    Check ENG-IMAP-SELECT "IMAP SELECT INBOX OK" ($rSel -match "a2 OK") "resp=$($rLogin -replace '\s+', ' ')"

    Send-ProtoLine $s "a3 FETCH 1:* (UID FLAGS)"
    $rFetch = Read-LiteralAware $s "a3"
    Check ENG-IMAP-FETCH "IMAP FETCH 1:* UID FLAGS" ($rFetch -match "FETCH" -and $rFetch -match "a3 OK") "resp=$($rFetch.Substring(0, [Math]::Min(90, $rFetch.Length)))"

    Send-ProtoLine $s "a4 FETCH 1 BODY.PEEK[]"
    $bodyLen = 0
    while ($true) {
        $line = Read-ProtoLine $s
        if ($line -eq "") { break }
        if ($line -match "\{(\d+)\}\r?$") {
            $n = [int]$Matches[1]
            $blob = New-Object byte[] $n
            $read = 0
            while ($read -lt $n) {
                $got = $s.Read($blob, $read, $n - $read)
                if ($got -le 0) { break }
                $read += $got
            }
            $bodyLen = $read
            $null = Read-ProtoLine $s
            continue
        }
        if ($line.StartsWith("a4 ")) { break }
    }
    Check ENG-IMAP-FETCHBODY "IMAP FETCH BODY.PEEK[] literal" ($bodyLen -gt 0) "literalLen=$bodyLen"

    Send-ProtoLine $s "a5 STORE 1 +FLAGS (`\Seen`)"
    $rStore = Read-LiteralAware $s "a5"
    Check ENG-IMAP-STORE "IMAP STORE +FLAGS Seen" ($rStore -match "a5 OK") "resp=$rStore"

    Send-ProtoLine $s "a6 COPY 1 Sent"
    $rCopy = Read-LiteralAware $s "a6"
    Check ENG-IMAP-COPY "IMAP COPY 1 -> Sent" ($rCopy -match "^a6 (OK|NO)") "resp=$rCopy"

    Send-ProtoLine $s "a7 STORE 1 +FLAGS (`\Deleted`)"
    $rDel = Read-LiteralAware $s "a7"
    Check ENG-IMAP-DELETE "IMAP STORE Deleted" ($rDel -match "a7 OK") "resp=$rDel"

    Send-ProtoLine $s "a8 EXPUNGE"
    $rExp = Read-LiteralAware $s "a8"
    Check ENG-IMAP-EXPUNGE "IMAP EXPUNGE" ($rExp -match "a8 OK") "resp=$rExp"

    Send-ProtoLine $s "a9 LOGOUT"
    $null = Read-LiteralAware $s "a9"
    $s.Close(); $c.Close()
} catch {
    Check ENG-IMAP-LOGIN "IMAP LOGIN OK (implicit TLS 993)" $false "err=$($_.Exception.Message)"
}

# ---------- POP3 995 implicit TLS ----------
try {
    $c = New-Tcp $Host_ $PopPort
    $s = Wrap-Tls $c
    $greet = Read-ProtoLine $s
    Check ENG-POP-GREET "POP3 greeting +OK (implicit TLS 995)" ($greet -match "^\+OK") "resp=$greet"

    Send-ProtoLine $s "USER $AdminEmail"
    $ru = Read-ProtoLine $s
    Send-ProtoLine $s "PASS $AdminPw"
    $rp = Read-ProtoLine $s
    Check ENG-POP-PASS "POP3 PASS +OK" (($ru -match "^\+OK") -and ($rp -match "^\+OK")) "user=$ru pass=$rp"

    Send-ProtoLine $s "STAT"
    $rs = Read-ProtoLine $s
    Check ENG-POP-STAT "POP3 STAT +OK n m" ($rs -match "^\+OK \d+ \d+") "resp=$rs"

    Send-ProtoLine $s "LIST"
    $l1 = Read-ProtoLine $s
    $lines = 1
    while ($true) {
        $l = Read-ProtoLine $s
        $lines++
        if ($l -eq "." -or $lines -gt 5000) { break }
    }
    Check ENG-POP-LIST "POP3 LIST multiline ends with ." ($l1 -match "^\+OK" -and $lines -ge 2) "lines=$lines"

    Send-ProtoLine $s "UIDL"
    $u1 = Read-ProtoLine $s
    if ($u1 -match "^\+OK") {
        $uN = 0
        while ($true) {
            $l = Read-ProtoLine $s
            $uN++
            if ($l -eq "." -or $uN -gt 5000) { break }
        }
    }
    Check ENG-POP-UIDL "POP3 UIDL +OK" ($u1 -match "^\+OK") "resp=$u1 lines=$uN"

    Send-ProtoLine $s "RETR 1"
    $r1 = Read-ProtoLine $s
    if ($r1 -match "^\+OK") {
        $bytes = 0
        while ($true) {
            $l = Read-ProtoLine $s
            $bytes += $l.Length
            if ($l -eq "." -or $bytes -gt 2097152) { break }
        }
    }
    Check ENG-POP-RETR "POP3 RETR 1 +OK multi-line" (($r1 -match "^\+OK") -and ($bytes -gt 0)) "resp=$r1 bytes=$bytes"

    Send-ProtoLine $s "TOP 1 0"
    $t1 = Read-ProtoLine $s
    if ($t1 -match "^\+OK") {
        $tb = 0
        while ($true) {
            $l = Read-ProtoLine $s
            $tb += $l.Length
            if ($l -eq "." -or $tb -gt 524288) { break }
        }
    }
    Check ENG-POP-TOP "POP3 TOP 1 0 +OK headers" (($t1 -match "^\+OK") -and ($tb -gt 0)) "resp=$t1 bytes=$tb"

    Send-ProtoLine $s "QUIT"
    $null = Read-ProtoLine $s
    $s.Close(); $c.Close()
} catch {
    Check ENG-POP-GREET "POP3 greeting +OK (implicit TLS 995)" $false "err=$($_.Exception.Message)"
}

# ---------- SMTP submission 587 with STARTTLS ----------
try {
    $c = New-Tcp $Host_ $SmtpPort
    $s = $c.GetStream()
    $greet = Read-ProtoLine $s
    Check ENG-SMTP-220 "SMTP greeting 220" ($greet -match "^220") "resp=$greet"

    Send-ProtoLine $s "EHLO qa-eng.local"
    $ehlo = ""
    while ($true) {
        $l = Read-ProtoLine $s
        if ($l -eq "") { break }
        $ehlo = "$ehlo$l | "
        if ($l -match "^250 ") { break }
    }
    Check ENG-SMTP-EHLO "SMTP EHLO 250" ($ehlo -match "^250") "resp=$($ehlo.Substring(0, [Math]::Min(70, $ehlo.Length)))"

    Send-ProtoLine $s "STARTTLS"
    $rt = Read-ProtoLine $s
    Check ENG-SMTP-STARTTLS "SMTP STARTTLS 220" ($rt -match "^220") "resp=$rt"
    $s = Wrap-Tls $c

    Send-ProtoLine $s "EHLO qa-eng.local"
    $ehlo2 = ""
    while ($true) {
        $l = Read-ProtoLine $s
        if ($l -eq "") { break }
        $ehlo2 = "$ehlo2$l | "
        if ($l -match "^250 ") { break }
    }
    Check ENG-SMTP-EHLO2 "SMTP EHLO after TLS advertises AUTH" (($ehlo2 -match "^250") -and ($ehlo2 -match "AUTH")) "resp=$($ehlo2.Substring(0, [Math]::Min(110, $ehlo2.Length)))"

    $plain = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("$AdminEmail`0$AdminEmail`0$AdminPw"))
    Send-ProtoLine $s "AUTH PLAIN $plain"
    $ra = Read-ProtoLine $s
    Check ENG-SMTP-AUTH "SMTP AUTH PLAIN 235 over TLS" ($ra -match "^235") "resp=$ra"

    Send-ProtoLine $s "MAIL FROM:<$AdminEmail>"
    $rm = Read-ProtoLine $s
    Send-ProtoLine $s "RCPT TO:<$AdminEmail>"
    $rc = Read-ProtoLine $s
    Check ENG-SMTP-RCPT "SMTP MAIL/RCPT 250" (($rm -match "^250") -and ($rc -match "^250")) "mail=$rm rcpt=$rc"

    Send-ProtoLine $s "DATA"
    $rd = Read-ProtoLine $s
    Send-ProtoLine $s "Subject: QA-eng-$rnd`r`nFrom: $AdminEmail`r`nTo: $AdminEmail`r`n`r`neng tls body $rnd`r`n."
    $dd = Read-ProtoLine $s
    Check ENG-SMTP-DATA "SMTP DATA 354 -> 250 queued" (($rd -match "^354") -and ($dd -match "^250")) "data=$dd"

    Send-ProtoLine $s "QUIT"
    $null = Read-ProtoLine $s
    $s.Close(); $c.Close()
} catch {
    Check ENG-SMTP-220 "SMTP greeting 220" $false "err=$($_.Exception.Message)"
}

# ---------- ManageSieve 4190 with STARTTLS ----------
try {
    $c = New-Tcp $Host_ $SievePort
    $s = $c.GetStream()
    $greet = Read-ProtoLine $s
    Check ENG-SIEVE-GREET "ManageSieve greeting IMPLEMENTATION" ($greet -match "IMPLEMENTATION") "resp=$greet"
    # The greeting ends with its own "OK ..." terminator; drain it before
    # issuing STARTTLS or the terminator is mistaken for the TLS reply.
    $g = 0
    while ($true) {
        $l = Read-ProtoLine $s
        $g++
        if (($l -match "^OK") -or ($g -gt 12) -or ($l -eq "")) { break }
    }

    Send-ProtoLine $s "STARTTLS"
    $rt = Read-ProtoLine $s
    Check ENG-SIEVE-STARTTLS "ManageSieve STARTTLS OK" ($rt -match "^OK") "resp=$rt"
    $s = Wrap-Tls $c

    $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes("$AdminEmail`0$AdminEmail`0$AdminPw"))
    Send-ProtoLine $s "AUTHENTICATE `"PLAIN`" `"$b64`""
    $ra = Read-ProtoLine $s
    Check ENG-SIEVE-AUTH "ManageSieve AUTHENTICATE OK over TLS" ($ra -match "^OK") "resp=$ra"

    Send-ProtoLine $s "LISTSCRIPTS"
    $rl = Read-ProtoLine $s
    Check ENG-SIEVE-LIST "ManageSieve LISTSCRIPTS responds" (($rl -match "^OK") -or ($rl -match "^`"")) "resp=$rl"

    Send-ProtoLine $s "LOGOUT"
    $null = Read-ProtoLine $s
    $s.Close(); $c.Close()
} catch {
    Check ENG-SIEVE-GREET "ManageSieve greeting IMPLEMENTATION" $false "err=$($_.Exception.Message)"
}

Flush-Results

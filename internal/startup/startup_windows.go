// Package startup manages only this user's Jonsbo Resurrection login entry.
package startup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

type Mode string

const (
	Disabled Mode = "disabled"
	Normal   Mode = "normal"
	Elevated Mode = "elevated"
)

var ErrElevationRequired = errors.New("administrator approval required to change elevated startup; run this command as administrator")

type snapshot struct {
	Run  string
	Task string
}
type backend interface {
	read() (snapshot, error)
	write(string, string) error
	sid() string
}

func State() (Mode, error) {
	b, err := newBackend()
	if err != nil {
		return Disabled, err
	}
	s, err := b.read()
	if err != nil {
		return Disabled, err
	}
	if s.Task != "" {
		return Elevated, nil
	}
	if s.Run != "" {
		return Normal, nil
	}
	return Disabled, nil
}
func Enable(exe string, elevated bool) error {
	if _, err := runCommand(exe); err != nil {
		return err
	}
	info, err := os.Stat(exe)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("startup executable is a directory")
	}
	b, err := newBackend()
	if err != nil {
		return err
	}
	m := Normal
	if elevated {
		m = Elevated
	}
	return change(b, exe, m)
}
func Disable() error {
	b, err := newBackend()
	if err != nil {
		return err
	}
	return change(b, "", Disabled)
}

func change(b backend, exe string, mode Mode) error {
	old, err := b.read()
	if err != nil {
		return err
	}
	desired := snapshot{}
	if mode == Normal {
		desired.Run, err = runCommand(exe)
		if err != nil {
			return err
		}
	}
	if mode == Elevated {
		if _, err = runCommand(exe); err != nil {
			return err
		}
		desired.Task = taskXML(exe, b.sid())
	}
	first, second := "run", "task"
	firstValue, secondValue := desired.Run, desired.Task
	oldFirst := old.Run
	if mode == Elevated {
		first, second = "task", "run"
		firstValue, secondValue = desired.Task, desired.Run
		oldFirst = old.Task
	}
	// Install the replacement before removing the previous mechanism.
	if err = b.write(first, firstValue); err != nil {
		return err
	}
	if err = b.write(second, secondValue); err != nil {
		if rollback := b.write(first, oldFirst); rollback != nil {
			return errors.Join(err, fmt.Errorf("startup rollback failed: %w", rollback))
		}
		return err
	}
	return nil
}
func runCommand(exe string) (string, error) {
	if !filepath.IsAbs(exe) || strings.HasPrefix(exe, `\\`) || strings.ContainsAny(exe, "\"\r\n\x00") || !strings.EqualFold(filepath.Ext(exe), ".exe") {
		return "", errors.New("startup needs an absolute local .exe path without quotes or control characters")
	}
	return `"` + exe + `" tray`, nil
}
func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
func taskXML(exe, sid string) string {
	return `<?xml version="1.0"?><Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><RegistrationInfo><Description>Jonsbo Resurrection managed startup v1</Description></RegistrationInfo><Triggers><LogonTrigger><Enabled>true</Enabled><UserId>` + escape(sid) + `</UserId></LogonTrigger></Triggers><Principals><Principal id="Author"><UserId>` + escape(sid) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals><Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><ExecutionTimeLimit>PT0S</ExecutionTimeLimit><Enabled>true</Enabled></Settings><Actions Context="Author"><Exec><Command>` + escape(exe) + `</Command><Arguments>tray</Arguments><WorkingDirectory>` + escape(filepath.Dir(exe)) + `</WorkingDirectory></Exec></Actions></Task>`
}

type windowsBackend struct{ userSID string }

func newBackend() (*windowsBackend, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(u.Uid, "S-1-") {
		return nil, errors.New("cannot determine Windows user SID")
	}
	return &windowsBackend{u.Uid}, nil
}
func (b *windowsBackend) sid() string { return b.userSID }
func (b *windowsBackend) read() (snapshot, error) {
	out, err := b.call("read", "")
	if err != nil {
		return snapshot{}, err
	}
	var s snapshot
	err = json.Unmarshal(out, &s)
	return s, err
}
func (b *windowsBackend) write(kind, value string) error { _, err := b.call(kind, value); return err }
func (b *windowsBackend) call(op, value string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]string{"op": op, "value": value, "sid": b.userSID})
	words := utf16.Encode([]rune(startupScript))
	raw := make([]byte, len(words)*2)
	for i, w := range words {
		binary.LittleEndian.PutUint16(raw[i*2:], w)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stdin = strings.NewReader(base64.StdEncoding.EncodeToString(payload))
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "JONSBO_ELEVATION_REQUIRED") {
			return nil, ErrElevationRequired
		}
		return nil, fmt.Errorf("startup %s: %w: %s", op, err, strings.TrimSpace(string(out)))
	}
	return bytes.TrimSpace(out), nil
}

// All dynamic input is JSON over stdin, never interpolated PowerShell source.
const startupScript = `
$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
[Console]::OutputEncoding=New-Object Text.UTF8Encoding($false)
try {
 $p=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String([Console]::In.ReadToEnd()))|ConvertFrom-Json
 $id=[Security.Principal.WindowsIdentity]::GetCurrent()
 if ($id.User.Value -ne $p.sid) { throw 'Startup user SID mismatch' }
 $name='JonsboResurrection-'+$p.sid
 $svc=New-Object -ComObject Schedule.Service
 $svc.Connect()
 $folder=$svc.GetFolder('\')
 $task=$null
 try { $task=$folder.GetTask($name) } catch {
  $cause=$_.Exception;while($cause.InnerException){$cause=$cause.InnerException}
  if ($cause.HResult -ne -2147024894 -and $cause.HResult -ne -2147024893) { throw }
 }
 if ($task -and $task.Definition.RegistrationInfo.Description -ne 'Jonsbo Resurrection managed startup v1') { throw 'Refusing to change an unrelated scheduled task with the same name' }
 $runKey=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Run',$false)
 $ownerKey=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\JonsboResurrection',$false)
 $run=$null; $owner=$null
 if ($runKey) { $run=$runKey.GetValue('JonsboResurrection',$null,[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames);$runKey.Close() }
 if ($ownerKey) { $owner=$ownerKey.GetValue('StartupCommand');$ownerKey.Close() }
 if ($null -ne $run -and ($run -isnot [string] -or $run -ne $owner)) { throw 'Refusing to change an unrelated Run value with the same name' }
 if ($p.op -eq 'read') { $tx='';if($task){$tx=$task.Xml};@{Run=[string]$run;Task=$tx}|ConvertTo-Json -Compress;exit 0 }
 if ($p.op -eq 'task') {
  if (!$task -and !$p.value) { exit 0 }
  $principal=New-Object Security.Principal.WindowsPrincipal($id)
  if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'JONSBO_ELEVATION_REQUIRED' }
  if ($p.value) { [void]$folder.RegisterTask($name,$p.value,6,$p.sid,$null,3,$null) } else { $folder.DeleteTask($name,0) }
 } elseif ($p.op -eq 'run') {
  if (!$run -and !$p.value) { exit 0 }
  $rk=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\Microsoft\Windows\CurrentVersion\Run')
  $ok=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\JonsboResurrection')
  try {
   if($p.value){$ok.SetValue('StartupCommand',$p.value,[Microsoft.Win32.RegistryValueKind]::String);$rk.SetValue('JonsboResurrection',$p.value,[Microsoft.Win32.RegistryValueKind]::String)}
   else{$rk.DeleteValue('JonsboResurrection',$false);$ok.DeleteValue('StartupCommand',$false)}
  } catch {
   $original=$_
   if($null -ne $run){$rk.SetValue('JonsboResurrection',$run,[Microsoft.Win32.RegistryValueKind]::String)}else{$rk.DeleteValue('JonsboResurrection',$false)}
   if($null -ne $owner){$ok.SetValue('StartupCommand',$owner)}else{$ok.DeleteValue('StartupCommand',$false)}
   throw $original
  }
  finally { $rk.Close();$ok.Close() }
 } else { throw 'Unknown startup operation' }
} catch { [Console]::Error.WriteLine($_.Exception.Message);exit 1 }
`

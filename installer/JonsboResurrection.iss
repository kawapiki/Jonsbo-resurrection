#ifndef AppVersion
  #error AppVersion is required
#endif
#ifndef PayloadDir
  #error PayloadDir is required
#endif
#ifndef OutputPath
  #error OutputPath is required
#endif

[Setup]
AppId={{51B2383D-E34E-4F8A-9BC2-B52D142D78B8}
AppName=Jonsbo Resurrection
AppVersion={#AppVersion}
AppPublisher=Csaba Kreth
AppPublisherURL=https://github.com/kawapiki/Jonsbo-resurrection
AppSupportURL=https://github.com/kawapiki/Jonsbo-resurrection/issues
AppUpdatesURL=https://github.com/kawapiki/Jonsbo-resurrection/releases/latest
DefaultDirName={localappdata}\Programs\JonsboResurrection
DefaultGroupName=Jonsbo Resurrection
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
OutputDir={#OutputPath}
OutputBaseFilename=JonsboResurrection-{#AppVersion}-Setup
VersionInfoVersion={#NumericVersion}
UninstallDisplayIcon={app}\JonsboResurrection.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
DisableWelcomePage=yes
DisableDirPage=yes
DisableProgramGroupPage=yes
DisableFinishedPage=yes
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

[Files]
Source: "{#PayloadDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#PayloadDir}\bin\jonsbo.exe"; DestName: "installer-helper.exe"; Flags: dontcopy

[Icons]
Name: "{group}\Jonsbo Resurrection"; Filename: "{app}\JonsboResurrection.exe"

[Code]
var
  StartupMode, UserSID: String;
  StartupFailed: Boolean;

function ReadLine(const Helper, Args: String; var Value: String): Boolean;
var
  Code: Integer;
  Output: TExecOutput;
begin
  Result := ExecAndCaptureOutput(Helper, Args, '', SW_HIDE, ewWaitUntilTerminated, Code, Output);
  Result := Result and (Code = 0) and not Output.Error and (GetArrayLength(Output.StdOut) = 1);
  if Result then Value := Trim(Output.StdOut[0]);
end;

function RunHelper(const Helper, Args: String; AllowElevation: Boolean): Boolean;
var
  Code: Integer;
begin
  Result := Exec(Helper, Args, '', SW_HIDE, ewWaitUntilTerminated, Code) and (Code = 0);
  if not Result and AllowElevation and not WizardSilent then
    Result := ShellExec('runas', Helper, Args, '', SW_HIDE, ewWaitUntilTerminated, Code) and (Code = 0);
end;

function StopTray(const Helper: String; Silent: Boolean): Boolean;
var
  Code, I: Integer;
  Args: String;
begin
  Result := True;
  if not CheckForMutexes('Local\JonsboResurrectionTray') then Exit;
  Args := 'stop --tray --expected-user-sid ' + UserSID;
  Result := Exec(Helper, Args, '', SW_HIDE, ewWaitUntilTerminated, Code) and (Code = 0);
  if not Result and not Silent then
    Result := ShellExec('runas', Helper, Args, '', SW_HIDE, ewWaitUntilTerminated, Code) and (Code = 0);
  if not Result then Exit;
  for I := 1 to 150 do begin
    if not CheckForMutexes('Local\JonsboResurrectionTray') then Exit;
    Sleep(100);
  end;
  Result := False;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Helper: String;
begin
  Result := '';
  ExtractTemporaryFile('installer-helper.exe');
  Helper := ExpandConstant('{tmp}\installer-helper.exe');
  if not ReadLine(Helper, 'startup identity', UserSID) or
     not ReadLine(Helper, 'startup status', StartupMode) then begin
    Result := 'Could not read your existing startup settings. No files have been changed.';
    Exit;
  end;
  if not StopTray(Helper, WizardSilent) then
    Result := 'Please exit Jonsbo Resurrection from its tray icon, then retry installation.';
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  Args: String;
  Code: Integer;
begin
  if CurStep = ssPostInstall then begin
    if (StartupMode = 'normal') or (StartupMode = 'elevated') then begin
      Args := 'startup enable';
      if StartupMode = 'elevated' then Args := Args + ' --elevated';
      Args := Args + ' --expected-user-sid ' + UserSID + ' --quiet';
      StartupFailed := not RunHelper(ExpandConstant('{app}\JonsboResurrection.exe'), Args, StartupMode = 'elevated');
      if StartupFailed then
        SuppressibleMsgBox('The app is installed, but Windows startup could not be updated. Your previous startup settings were kept. Enable startup from the new app to finish the upgrade.', mbError, MB_OK, IDOK);
    end;
  end;
  if (CurStep = ssDone) and not WizardSilent and not StartupFailed then begin
    if StartupMode = 'elevated' then begin
      if not Exec(ExpandConstant('{sys}\schtasks.exe'), '/Run /TN "JonsboResurrection-' + UserSID + '"', '', SW_HIDE, ewWaitUntilTerminated, Code) or (Code <> 0) then
        SuppressibleMsgBox('Installation finished. Open Jonsbo Resurrection from the Start menu to start monitoring.', mbInformation, MB_OK, IDOK);
    end else
      ExecAsOriginalUser(ExpandConstant('{app}\JonsboResurrection.exe'), 'tray', '', SW_SHOWNORMAL, ewNoWait, Code);
  end;
end;

function GetCustomSetupExitCode: Integer;
begin
  Result := 0;
  if StartupFailed then Result := 1;
end;

function PrepareUninstall: Boolean;
var
  Helper, Args: String;
  Code: Integer;
begin
  Helper := ExpandConstant('{app}\bin\jonsbo.exe');
  Result := ReadLine(Helper, 'startup identity', UserSID);
  if Result then Result := StopTray(Helper, UninstallSilent);
  if Result then begin
    Args := 'startup disable-owned --expected-user-sid ' + UserSID + ' --quiet';
    Result := Exec(ExpandConstant('{app}\JonsboResurrection.exe'), Args, '', SW_HIDE, ewWaitUntilTerminated, Code) and (Code = 0);
    if not Result and not UninstallSilent then
      Result := ShellExec('runas', ExpandConstant('{app}\JonsboResurrection.exe'), Args, '', SW_HIDE, ewWaitUntilTerminated, Code) and (Code = 0);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    if not PrepareUninstall then
      RaiseException('Could not close the app or remove its startup entry. Uninstallation was cancelled; your files and settings were kept.');
end;

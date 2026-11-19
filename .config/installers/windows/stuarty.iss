; Inno Setup script of the Windows installer (SDD-009 section 10.4, P1-REL-010).
;
; Build:  ISCC.exe /DAppVersion=0.2.0 /DSourceBinary=<stuarty.exe> /DOutputDir=<dir> stuarty.iss
;
; The installer is per user by default (no administrator prompt, no paid signing): the program goes to
; %LOCALAPPDATA%\Programs\Stuarty, and the user may choose an all-users install in the first page.
; It puts stuarty on the PATH (a task, on by default) and adds a Start menu shortcut that runs
; "stuarty open": start the local server on 127.0.0.1 if none runs, then open the browser.
; Uninstall removes the program and the shortcuts and keeps the workspace (%USERPROFILE%\.stuarty,
; or STUARTY_HOME): the user's projects and media are never deleted by an uninstall.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceBinary
  #error SourceBinary is required: the path of stuarty.exe
#endif
#ifndef OutputDir
  #define OutputDir "."
#endif
#define AppName "Stuarty"
#define AppExe "stuarty.exe"
#define FormerExe "vukem-video.exe"

[Setup]
; A fixed AppId lets a new version upgrade the old one in place.
AppId={{7C5C2F0E-3B4A-4D8B-9E21-6A1F0D4C8E55}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=Stuarty
AppPublisherURL=https://github.com/AlexGalhardo/stuarty
AppSupportURL=https://github.com/AlexGalhardo/stuarty/issues
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
ChangesEnvironment=yes
LicenseFile=..\..\..\LICENSE.md
SetupIconFile=..\assets\stuarty.ico
UninstallDisplayIcon={app}\stuarty.ico
UninstallDisplayName={#AppName}
OutputDir={#OutputDir}
#ifndef FileVersion
  #define FileVersion "0-0-0"
#endif
; FileVersion is AppVersion with dots replaced by dashes: stuarty-0-15-0.exe.
OutputBaseFilename=stuarty-{#FileVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "addtopath"; Description: "Add stuarty to the PATH, to use it from a terminal"; GroupDescription: "Command line:"
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts:"; Flags: unchecked

[Files]
Source: "{#SourceBinary}"; DestDir: "{app}"; DestName: "{#AppExe}"; Flags: ignoreversion
; The former command (ADR-0044, SDD-009 section 10.5): a copy of the program named as before, so that what called
; vukem-video (an agent MCP config, a script) keeps working after an upgrade of an installed Vukem Video.
Source: "{#SourceBinary}"; DestDir: "{app}"; DestName: "{#FormerExe}"; Flags: ignoreversion
Source: "..\assets\stuarty.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\LICENSE.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\licenses\*"; DestDir: "{app}\licenses"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExe}"; Parameters: "open"; IconFilename: "{app}\stuarty.ico"; Comment: "Start the local server and open Stuarty in your browser"; Flags: runminimized
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Parameters: "open"; IconFilename: "{app}\stuarty.ico"; Tasks: desktopicon; Flags: runminimized

[InstallDelete]
; An upgrade of an installed Vukem Video (same AppId, so the same folder): remove what carried the former name.
; Only program files and shortcuts, never the workspace (SDD-009 section 10.5).
Type: files; Name: "{app}\vukem-video.ico"
Type: files; Name: "{autoprograms}\Vukem Video.lnk"
Type: files; Name: "{autodesktop}\Vukem Video.lnk"

[Run]
Filename: "{app}\{#AppExe}"; Parameters: "open"; Description: "Open {#AppName} now"; Flags: postinstall nowait skipifsilent runminimized

[UninstallRun]
; The local server runs in the background: stop it so the program can be removed. The workspace is not touched.
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#AppExe}"; Flags: runhidden; RunOnceId: "StopLocalServer"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#FormerExe}"; Flags: runhidden; RunOnceId: "StopFormerServer"

[Code]
const
  UserEnvKey = 'Environment';
  MachineEnvKey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';

function EnvRoot(): Integer;
begin
  if IsAdminInstallMode then
    Result := HKEY_LOCAL_MACHINE
  else
    Result := HKEY_CURRENT_USER;
end;

function EnvKey(): String;
begin
  if IsAdminInstallMode then
    Result := MachineEnvKey
  else
    Result := UserEnvKey;
end;

function PathContains(const Path, Dir: String): Boolean;
begin
  Result := Pos(';' + Lowercase(Dir) + ';', ';' + Lowercase(Path) + ';') > 0;
end;

procedure AddToPath();
var
  Path: String;
begin
  if not RegQueryStringValue(EnvRoot(), EnvKey(), 'Path', Path) then
    Path := '';
  if PathContains(Path, ExpandConstant('{app}')) then
    Exit;
  if (Path <> '') and (Path[Length(Path)] <> ';') then
    Path := Path + ';';
  RegWriteExpandStringValue(EnvRoot(), EnvKey(), 'Path', Path + ExpandConstant('{app}'));
end;

procedure RemoveFromPath();
var
  Path, Dir: String;
  P: Integer;
begin
  if not RegQueryStringValue(EnvRoot(), EnvKey(), 'Path', Path) then
    Exit;
  Dir := ExpandConstant('{app}');
  Path := ';' + Path + ';';
  P := Pos(';' + Lowercase(Dir) + ';', Lowercase(Path));
  if P = 0 then
    Exit;
  Delete(Path, P, Length(Dir) + 1);
  if (Path <> '') and (Path[1] = ';') then
    Delete(Path, 1, 1);
  if (Path <> '') and (Path[Length(Path)] = ';') then
    Delete(Path, Length(Path), 1);
  RegWriteExpandStringValue(EnvRoot(), EnvKey(), 'Path', Path);
end;

// An upgrade or a reinstall replaces the program: stop the background server first, or Windows keeps
// the file locked.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#AppExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  // The server of an installed Vukem Video owns its workspace until it stops, and locks its file.
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#FormerExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Result := '';
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and WizardIsTaskSelected('addtopath') then
    AddToPath();
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath();
end;

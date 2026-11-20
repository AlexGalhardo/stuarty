; Inno Setup script of the Windows installer of the desktop application (SDD-009 sections 10.4 and 10.5,
; P3-REL-003, D-033).
;
; Build:  ISCC.exe /DAppVersion=0.15.0 /DFileVersion=0-15-0 /DDesktopBinary=<stuarty-desktop.exe>
;                  /DServerBinary=<stuarty.exe> /DOutputDir=<dir> stuarty-desktop.iss
; FileVersion is AppVersion with dots replaced by dashes, so the download is
; stuarty-desktop-0-15-0.exe.
;
; The installer is per user by default, with no administrator prompt and no paid signing (D-033): Windows
; SmartScreen shows "Windows protected your PC" until the user chooses "More info" then "Run anyway". The
; desktop application and the server it starts (it looks for stuarty.exe in its own folder) go to
; %LOCALAPPDATA%\Programs\Stuarty Desktop. A Start menu shortcut runs the desktop application.
; Uninstall removes the program and the shortcuts and keeps the workspace (%USERPROFILE%\.stuarty, or
; STUARTY_HOME): the user's projects and media are never deleted by an uninstall.
;
; Background mode (epic 21, SDD-012 section 21.3 item 7): when the person turns on "Start when I sign in",
; the application writes the value StuartyDesktop in HKCU\Software\Microsoft\Windows\CurrentVersion\Run (the
; value and the key are desktop/service RunValueName and RunSubKey; a test keeps this file in step with
; them). Nothing here writes it: the installer never registers anything that starts by itself. The uninstaller
; deletes the value, twice over ([Registry] and [UninstallRun]) so that no registration is left behind, and
; stops the resident application, whose window may be hidden, before it removes the files.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef DesktopBinary
  #error DesktopBinary is required: the path of stuarty-desktop.exe
#endif
#ifndef ServerBinary
  #error ServerBinary is required: the path of stuarty.exe
#endif
#ifndef OutputDir
  #define OutputDir "."
#endif
#ifndef FileVersion
  #define FileVersion "0-0-0"
#endif
#define AppName "Stuarty Desktop"
#define AppExe "stuarty-desktop.exe"
#define ServerExe "stuarty.exe"
#define FormerAppExe "vukem-video-desktop.exe"
#define FormerServerExe "vukem-video.exe"

[Setup]
; A fixed AppId, other than the one of the server installer, lets a new version upgrade this one in place.
AppId={{B4E0A6D2-5F1C-4C37-8A9B-2D7E3F61C0A9}
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
LicenseFile=..\..\..\LICENSE.md
SetupIconFile=..\..\..\.config\installers\assets\stuarty.ico
UninstallDisplayIcon={app}\stuarty.ico
UninstallDisplayName={#AppName}
OutputDir={#OutputDir}
OutputBaseFilename=stuarty-desktop-{#FileVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
; An update started by the application: the running desktop window and its server are closed first.
CloseApplications=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Shortcuts:"; Flags: unchecked

[Files]
Source: "{#DesktopBinary}"; DestDir: "{app}"; DestName: "{#AppExe}"; Flags: ignoreversion
Source: "{#ServerBinary}"; DestDir: "{app}"; DestName: "{#ServerExe}"; Flags: ignoreversion
Source: "..\..\..\.config\installers\assets\stuarty.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\LICENSE.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\licenses\*"; DestDir: "{app}\licenses"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\stuarty.ico"; Comment: "Open the Stuarty desktop application"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\stuarty.ico"; Tasks: desktopicon

[Registry]
; Written by the application when the person turns on "Start when I sign in", never by the installer: with
; ValueType none Setup creates nothing, and the flags delete the value when the program is uninstalled.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueName: "StuartyDesktop"; ValueType: none; Flags: dontcreatekey uninsdeletevalue

[InstallDelete]
; An upgrade of an installed Vukem Video Desktop (same AppId, so the same folder): remove what carried the former
; name. Only program files and shortcuts, never the workspace (SDD-009 section 10.5).
Type: files; Name: "{app}\{#FormerAppExe}"
Type: files; Name: "{app}\{#FormerServerExe}"
Type: files; Name: "{app}\vukem-video.ico"
Type: files; Name: "{autoprograms}\Vukem Video Desktop.lnk"
Type: files; Name: "{autodesktop}\Vukem Video Desktop.lnk"

[Run]
Filename: "{app}\{#AppExe}"; Description: "Open {#AppName} now"; Flags: postinstall nowait skipifsilent

[UninstallRun]
; The start at sign-in is removed first, so a stopped application is never started again by the next sign-in
; (the [Registry] entry does the same; a value that is already gone is not an error).
Filename: "{sys}\reg.exe"; Parameters: "delete ""HKCU\Software\Microsoft\Windows\CurrentVersion\Run"" /v StuartyDesktop /f"; Flags: runhidden; RunOnceId: "RemoveStartAtSignIn"
; The server the application started may outlive it: stop both so the program can be removed. The workspace is not touched.
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#AppExe}"; Flags: runhidden; RunOnceId: "StopDesktop"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#ServerExe}"; Flags: runhidden; RunOnceId: "StopServer"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#FormerAppExe}"; Flags: runhidden; RunOnceId: "StopFormerDesktop"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#FormerServerExe}"; Flags: runhidden; RunOnceId: "StopFormerServer"

[Code]
// An upgrade or a reinstall replaces the program: stop the application and its server first, or Windows
// keeps the files locked.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#AppExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#ServerExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  // An installed Vukem Video Desktop and its server own the workspace until they stop.
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#FormerAppExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#FormerServerExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Result := '';
end;

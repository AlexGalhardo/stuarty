; Inno Setup script of the Windows installer of the desktop application (SDD-009 sections 10.4 and 10.5,
; P3-REL-003, D-033).
;
; Build:  ISCC.exe /DAppVersion=0.15.0 /DFileVersion=0-15-0 /DDesktopBinary=<vukem-video-desktop.exe>
;                  /DServerBinary=<vukem-video.exe> /DOutputDir=<dir> vukem-video-desktop.iss
; FileVersion is AppVersion with dots replaced by dashes, so the download is
; vukem-video-desktop-0-15-0.exe.
;
; The installer is per user by default, with no administrator prompt and no paid signing (D-033): Windows
; SmartScreen shows "Windows protected your PC" until the user chooses "More info" then "Run anyway". The
; desktop application and the server it starts (it looks for vukem-video.exe in its own folder) go to
; %LOCALAPPDATA%\Programs\Vukem Video Desktop. A Start menu shortcut runs the desktop application.
; Uninstall removes the program and the shortcuts and keeps the workspace (%USERPROFILE%\.vukem-video, or
; VUKEM_VIDEO_HOME): the user's projects and media are never deleted by an uninstall.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef DesktopBinary
  #error DesktopBinary is required: the path of vukem-video-desktop.exe
#endif
#ifndef ServerBinary
  #error ServerBinary is required: the path of vukem-video.exe
#endif
#ifndef OutputDir
  #define OutputDir "."
#endif
#ifndef FileVersion
  #define FileVersion "0-0-0"
#endif
#define AppName "Vukem Video Desktop"
#define AppExe "vukem-video-desktop.exe"
#define ServerExe "vukem-video.exe"

[Setup]
; A fixed AppId, other than the one of the server installer, lets a new version upgrade this one in place.
AppId={{B4E0A6D2-5F1C-4C37-8A9B-2D7E3F61C0A9}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=Vukem Video
AppPublisherURL=https://github.com/AlexGalhardo/vukem-video
AppSupportURL=https://github.com/AlexGalhardo/vukem-video/issues
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
LicenseFile=..\..\..\LICENSE.md
SetupIconFile=..\..\..\.config\installers\assets\vukem-video.ico
UninstallDisplayIcon={app}\vukem-video.ico
UninstallDisplayName={#AppName}
OutputDir={#OutputDir}
OutputBaseFilename=vukem-video-desktop-{#FileVersion}
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
Source: "..\..\..\.config\installers\assets\vukem-video.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\LICENSE.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\THIRD_PARTY_NOTICES.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\..\licenses\*"; DestDir: "{app}\licenses"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\vukem-video.ico"; Comment: "Open the Vukem Video desktop application"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; IconFilename: "{app}\vukem-video.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\{#AppExe}"; Description: "Open {#AppName} now"; Flags: postinstall nowait skipifsilent

[UninstallRun]
; The server the application started may outlive it: stop both so the program can be removed. The workspace is not touched.
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#AppExe}"; Flags: runhidden; RunOnceId: "StopDesktop"
Filename: "{sys}\taskkill.exe"; Parameters: "/F /IM {#ServerExe}"; Flags: runhidden; RunOnceId: "StopServer"

[Code]
// An upgrade or a reinstall replaces the program: stop the application and its server first, or Windows
// keeps the files locked.
function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  Code: Integer;
begin
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#AppExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Exec(ExpandConstant('{sys}\taskkill.exe'), '/F /IM {#ServerExe}', '', SW_HIDE, ewWaitUntilTerminated, Code);
  Result := '';
end;

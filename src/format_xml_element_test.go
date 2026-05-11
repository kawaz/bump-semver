package main

import (
	"strings"
	"testing"
)

// --- pom.xml (Maven) -----------------------------------------------------

func TestXmlElement_PomXml_Inspect(t *testing.T) {
	t.Parallel()
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <groupId>com.example</groupId>
    <artifactId>my-app</artifactId>
    <version>1.2.3</version>
</project>
`)
	insp, err := inspectVia("pom.xml", in)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
	if len(insp.Names) == 0 || insp.Names[0].Value != "my-app" {
		t.Errorf("Names = %+v", insp.Names)
	}
}

// Parent-version must not be picked up — the rule asks for
// /project/version which is at depth 2, parent's version is at depth 3.
func TestXmlElement_PomXml_ParentVersionIgnored(t *testing.T) {
	t.Parallel()
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>com.example</groupId>
        <artifactId>parent-pom</artifactId>
        <version>9.9.9</version>
    </parent>
    <artifactId>my-app</artifactId>
    <version>1.2.3</version>
</project>
`)
	insp, err := inspectVia("pom.xml", in)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("expected child version 1.2.3, got %+v (parent leaked?)", insp.Versions)
	}
}

func TestXmlElement_PomXml_Replace(t *testing.T) {
	t.Parallel()
	in := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
    <parent>
        <version>9.9.9</version>
    </parent>
    <version>1.2.3</version>
</project>
`)
	out, err := replaceVia("pom.xml", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !strings.Contains(string(out), "<version>1.2.4</version>") {
		t.Errorf("Replace did not write 1.2.4:\n%s", string(out))
	}
	if !strings.Contains(string(out), "<version>9.9.9</version>") {
		t.Errorf("Parent version was accidentally rewritten:\n%s", string(out))
	}
}

// --- *.csproj / *.fsproj / *.vbproj (.NET MSBuild) ----------------------

func TestXmlElement_Csproj_Inspect(t *testing.T) {
	t.Parallel()
	in := []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <Version>1.2.3</Version>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>
`)
	insp, err := inspectVia("MyApp.csproj", in)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

func TestXmlElement_Csproj_Replace(t *testing.T) {
	t.Parallel()
	in := []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <Version>1.2.3</Version>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>
`)
	out, err := replaceVia("MyApp.csproj", in, "1.2.3", "1.2.4")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !strings.Contains(string(out), "<Version>1.2.4</Version>") {
		t.Errorf("Replace did not write 1.2.4:\n%s", string(out))
	}
	// TargetFramework left untouched.
	if !strings.Contains(string(out), "<TargetFramework>net8.0</TargetFramework>") {
		t.Errorf("TargetFramework was unexpectedly modified:\n%s", string(out))
	}
}

func TestXmlElement_Fsproj_Inspect(t *testing.T) {
	t.Parallel()
	in := []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <Version>3.4.5</Version>
  </PropertyGroup>
</Project>
`)
	insp, err := inspectVia("MyLib.fsproj", in)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Versions[0].Value != "3.4.5" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

// Multiple PropertyGroup blocks: first match wins (the leftmost
// /Project/PropertyGroup/Version in document order).
func TestXmlElement_Csproj_MultiplePropertyGroups(t *testing.T) {
	t.Parallel()
	in := []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <Version>1.2.3</Version>
  </PropertyGroup>
  <PropertyGroup Condition="'$(Configuration)' == 'Release'">
    <Optimize>true</Optimize>
  </PropertyGroup>
</Project>
`)
	insp, err := inspectVia("MyApp.csproj", in)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Versions[0].Value != "1.2.3" {
		t.Errorf("Versions = %+v", insp.Versions)
	}
}

// Missing version: rule should fall through.
func TestXmlElement_Csproj_MissingVersion(t *testing.T) {
	t.Parallel()
	in := []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>
`)
	if _, err := inspectVia("MyApp.csproj", in); err == nil {
		t.Error("expected error for missing <Version>")
	}
}

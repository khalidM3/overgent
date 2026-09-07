import { Folder, FolderOpen } from "lucide-react";

/**
 * A Project is anchored to a repository folder, so its navigation mark says
 * that directly instead of manufacturing an ambiguous initial from its name.
 */
export function ProjectIcon({ selected = false }: { selected?: boolean }) {
  const Icon = selected ? FolderOpen : Folder;
  return <span className="project-icon" aria-hidden="true"><Icon size={17} strokeWidth={1.7} /></span>;
}

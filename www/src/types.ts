export interface Container {
  id: string;
  names: string[];
  image: string;
  state: string;
  status: string;
  created: number;
  stats?: ContainerStats;
}

export interface ContainerStats {
  memory_stats: {
    usage: number;
    limit: number;
  };
  cpu_stats: {
    usage: number;
    cores: number;
    system_ms: number;
  };
}

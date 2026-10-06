"""Population-moment statistics for sampled observables."""

def summarize_samples(samples, n_sites, temperature):
    """Per-spin means and response functions using population moments.

    heat_capacity_per_spin  = Var(E) / (N * T^2)
    susceptibility_per_spin = Var(M) / (N * T)
    Variances use population moments (divide by count, not count - 1).
    """
    if not samples:
        raise ValueError("no samples to summarize")
    count = len(samples)
    mean_e = sum(s.energy for s in samples) / count
    mean_m = sum(s.magnetization for s in samples) / count
    mean_abs_m = sum(abs(s.magnetization) for s in samples) / count
    var_e = sum((s.energy - mean_e) ** 2 for s in samples) / count
    var_m = sum((s.magnetization - mean_m) ** 2 for s in samples) / count
    return {
        "n_samples": count,
        "energy_per_spin": mean_e / n_sites,
        "abs_magnetization_per_spin": mean_abs_m / n_sites,
        "magnetization_per_spin": mean_m / n_sites,
        "energy_variance": var_e,
        "magnetization_variance": var_m,
        "heat_capacity_per_spin": var_e / (n_sites * temperature * temperature),
        "susceptibility_per_spin": var_m / (n_sites * temperature),
    }

"""Global one-to-one assignment with gating and unmatched penalties.

* block (track, observation): squared Mahalanobis distance when the
  association passes the gate, otherwise a large forbidden cost;
* block (track, dummy column of that track): missed_penalty, i.e. the
  track stays unmatched this frame;
* block (dummy row of that observation, observation): new_track_penalty,
  i.e. the observation spawns a new track;
* dummy-dummy block: zero.

The square matrix of size n_tracks + n_observations is solved exactly
with the Hungarian (Kuhn-Munkres) algorithm, guaranteeing a global
minimum of the total cost instead of greedy per-track claims.
Determinism: tracks are processed in ascending track id order and
observations in ascending observation id order; ties are resolved by
the Hungarian algorithm's fixed row scan order, so reordering
observations within a frame cannot change the result.
"""

from __future__ import annotations

FORBIDDEN = 1.0e18


def _hungarian(cost):
    """Min-cost perfect matching on a square matrix.

    Returns a list `match` where match[i] is the column assigned to
    row i. Deterministic for fixed input ordering.
    """
    n = len(cost)
    u = [0.0] * (n + 1)
    v = [0.0] * (n + 1)
    p = [0] * (n + 1)
    way = [0] * (n + 1)
    for i in range(1, n + 1):
        p[0] = i
        j0 = 0
        minv = [float("inf")] * (n + 1)
        used = [False] * (n + 1)
        while True:
            used[j0] = True
            i0 = p[j0]
            delta = float("inf")
            j1 = 0
            for j in range(1, n + 1):
                if used[j]:
                    continue
                cur = cost[i0 - 1][j - 1] - u[i0] - v[j]
                if cur < minv[j]:
                    minv[j] = cur
                    way[j] = j0
                if minv[j] < delta:
                    delta = minv[j]
                    j1 = j
            for j in range(n + 1):
                if used[j]:
                    u[p[j]] += delta
                    v[j] -= delta
                else:
                    minv[j] -= delta
            j0 = j1
            if p[j0] == 0:
                break
        while j0:
            j1 = way[j0]
            p[j0] = p[j1]
            j0 = j1
    match = [None] * n
    for j in range(1, n + 1):
        if p[j] != 0:
            match[p[j] - 1] = j - 1
    return match


def assign(gate_costs, n_obs, missed_penalty, new_track_penalty):
    """Solve the global assignment.

    gate_costs: n_tracks x n_obs matrix; entry is the squared
        Mahalanobis distance or None when gated out. n_obs is passed
        explicitly because it cannot be inferred when n_tracks is 0.
    Returns (matches, unmatched_tracks, unmatched_observations) where
    matches is a list of (track_index, obs_index, cost).
    """
    n_tracks = len(gate_costs)
    n = n_tracks + n_obs
    if n == 0:
        return [], [], []
    cost = [[FORBIDDEN] * n for _ in range(n)]
    for t in range(n_tracks):
        for o in range(n_obs):
            value = gate_costs[t][o]
            cost[t][o] = value if value is not None else FORBIDDEN
        cost[t][n_obs + t] = missed_penalty
    for o in range(n_obs):
        cost[n_tracks + o][o] = new_track_penalty
    for i in range(n_tracks, n):
        for j in range(n_obs, n):
            cost[i][j] = 0.0
    match = _hungarian(cost)
    matches = []
    unmatched_tracks = []
    matched_obs = set()
    for t in range(n_tracks):
        col = match[t]
        if col < n_obs and gate_costs[t][col] is not None:
            matches.append((t, col, gate_costs[t][col]))
            matched_obs.add(col)
        else:
            unmatched_tracks.append(t)
    unmatched_obs = [o for o in range(n_obs) if o not in matched_obs]
    return matches, unmatched_tracks, unmatched_obs

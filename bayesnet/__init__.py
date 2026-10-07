"""Discrete Bayesian network query tool (stdlib only)."""

from .model import BayesianNetwork, load_network
from .inference import query, QueryError

__all__ = ["BayesianNetwork", "load_network", "query", "QueryError"]

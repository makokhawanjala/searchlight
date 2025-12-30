# API Documentation

SearchLight provides a RESTful API for searching and managing the file index.

## Endpoints

### Search
```
GET /api/search?q=query&ext=.txt
```

### Statistics
```
GET /api/stats
```

### Rebuild Index
```
POST /api/index/rebuild
```

### Health Checks
```
GET /health
GET /ready
```

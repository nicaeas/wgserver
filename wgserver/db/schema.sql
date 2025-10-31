-- Database schema for wgserver
CREATE DATABASE IF NOT EXISTS wgserver CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE wgserver;

-- clients
CREATE TABLE IF NOT EXISTS clients (
  id VARCHAR(64) PRIMARY KEY,
  zone VARCHAR(128) DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

-- roles
CREATE TABLE IF NOT EXISTS roles (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  role_name VARCHAR(128) NOT NULL,
  zone VARCHAR(128) NOT NULL,
  merge_state VARCHAR(32) NOT NULL,
  class VARCHAR(32) NOT NULL,
  school VARCHAR(32) DEFAULT '',
  skill INT DEFAULT 0,
  level INT DEFAULT 0,
  lucky INT DEFAULT 0,
  magic INT DEFAULT 0,
  current_map VARCHAR(128) DEFAULT '',
  client_id VARCHAR(64) DEFAULT '',
  created_at TIMESTAMP NULL DEFAULT NULL,
  x INT DEFAULT 0,
  y INT DEFAULT 0,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_zone_role (zone, role_name)
) ENGINE=InnoDB;

-- equipments inventory (pooled per zone, with owner role if any)
CREATE TABLE IF NOT EXISTS equipments (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  zone VARCHAR(128) NOT NULL,
  item_name VARCHAR(128) NOT NULL,
  slot VARCHAR(32) DEFAULT '',
  item_level INT DEFAULT 0,
  enhance INT DEFAULT 0,
  refine INT DEFAULT 0,
  count INT DEFAULT 1,
  owner_role VARCHAR(128) DEFAULT NULL,
  location ENUM('equipped','bag','warehouse') NOT NULL,
  UNIQUE KEY uk_zone_item_owner (zone, item_name, owner_role, location)
) ENGINE=InnoDB;

-- role_equipment snapshot (assigned result after planning)
CREATE TABLE IF NOT EXISTS role_equipment (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  zone VARCHAR(128) NOT NULL,
  role_name VARCHAR(128) NOT NULL,
  slot VARCHAR(32) NOT NULL,
  item_name VARCHAR(128) NOT NULL,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_zone_role_slot (zone, role_name, slot)
) ENGINE=InnoDB;

-- map allocations
CREATE TABLE IF NOT EXISTS map_allocations (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  zone VARCHAR(128) NOT NULL,
  role_name VARCHAR(128) NOT NULL,
  map_name VARCHAR(128) NOT NULL,
  floor INT DEFAULT 1,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_zone_role (zone, role_name)
) ENGINE=InnoDB;

-- daily tasks queue per zone
CREATE TABLE IF NOT EXISTS daily_tasks (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  zone VARCHAR(128) NOT NULL,
  role_name VARCHAR(128) NOT NULL,
  status ENUM('waiting','running','done') NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_zone_status (zone, status)
) ENGINE=InnoDB;

-- equipment exchanges
CREATE TABLE IF NOT EXISTS exchanges (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  zone VARCHAR(128) NOT NULL,
  owner_role VARCHAR(128) NOT NULL,
  receiver_role VARCHAR(128) NOT NULL,
  item_name VARCHAR(128) NOT NULL,
  status ENUM('waiting','owner_ok','receiver_ok','done','aborted') NOT NULL DEFAULT 'waiting',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_zone_status (zone, status)
) ENGINE=InnoDB;

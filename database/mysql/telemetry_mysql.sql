SET SQL_MODE = "NO_AUTO_VALUE_ON_ZERO";
SET AUTOCOMMIT = 0;
START TRANSACTION;
SET time_zone = "+00:00";


/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8mb4 */;

--
-- Database: `speedtest_telemetry`
--

-- --------------------------------------------------------

--
-- Table structure for table `speedtest_users`
--

CREATE TABLE `speedtest_users` (
  `id` int(11) NOT NULL,
  `timestamp` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `ip` text NOT NULL,
  `ispinfo` text,
  `extra` text,
  `ua` text NOT NULL,
  `lang` text NOT NULL,
  `dl` text,
  `ul` text,
  `ping` text,
  `jitter` text,
  `log` longtext,
  `uuid` text,
  `grade_data` text,
  `chart_data` longtext,
  `latency_underload` text,
  `ping_during_test` longtext,
  `client_id` text
) ENGINE=InnoDB DEFAULT CHARSET=latin1;

--
-- Migration for existing installs: add the columns above if they are
-- missing (MySQL 8.0.29+ / MariaDB 10.5.2+ support IF NOT EXISTS here;
-- on older versions, drop the clause and ignore the "duplicate column"
-- error if you re-run this).
--
-- ALTER TABLE `speedtest_users` ADD COLUMN IF NOT EXISTS `grade_data` text;
-- ALTER TABLE `speedtest_users` ADD COLUMN IF NOT EXISTS `chart_data` longtext;
-- ALTER TABLE `speedtest_users` ADD COLUMN IF NOT EXISTS `latency_underload` text;
-- ALTER TABLE `speedtest_users` ADD COLUMN IF NOT EXISTS `ping_during_test` longtext;
-- ALTER TABLE `speedtest_users` ADD COLUMN IF NOT EXISTS `client_id` text;

--
-- Indexes for dumped tables
--

--
-- Indexes for table `speedtest_users`
--
ALTER TABLE `speedtest_users`
  ADD PRIMARY KEY (`id`);

--
-- AUTO_INCREMENT for dumped tables
--

--
-- AUTO_INCREMENT for table `speedtest_users`
--
ALTER TABLE `speedtest_users`
  MODIFY `id` int(11) NOT NULL AUTO_INCREMENT;COMMIT;

/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;

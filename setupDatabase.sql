DROP TABLE IF EXISTS inprogress;
CREATE TABLE inprogress (PickupId SERIAL,
                       PhoneNumber CHAR(10) NOT NULL,
                       DeviceId VARCHAR(36) NOT NULL,
                       InitialLatitude REAL NOT NULL,
                       InitialLongitude REAL NOT NULL,
                       InitialTime TIMESTAMP NOT NULL,
                       LatestLatitude REAL NOT NULL,
                       LatestLongitude REAL NOT NULL,
                       LatestTime TIMESTAMP NOT NULL,
                       ConfirmTime TIMESTAMP NOT NULL,
                       CompleteTime TIMESTAMP NOT NULL,
                       Status INT NOT NULL,
                       CONSTRAINT PK_PickupId PRIMARY KEY (PickupId),
                       CONSTRAINT Check_PhoneNumber CHECK (CHAR_LENGTH(PhoneNumber) = 10));

#View public schema tables
SELECT table_schema,table_name
FROM information_schema.tables
WHERE table_schema = 'public'
ORDER BY table_schema,table_name;

#Check for table existence
SELECT EXISTS (
   SELECT 1
   FROM   information_schema.tables 
   WHERE  table_schema = 'public'
   AND    table_name = 'inprogress'
);

#View inprogress table columns
SELECT *
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name   = 'inprogress'

#Insert new pickup
INSERT INTO inprogress (PhoneNumber, DeviceId, InitialLatitude, InitialLongitude, InitialTime, LatestLatitude, LatestLongitude, LatestTime, ConfirmTime, CompleteTime, Status)
  VALUES ('5103868680', '68753A44-4D6F-1226-9C60-0050E4C00067', 38.9844, 76.4889, '2002-10-02T10:00:00-05:00', 38.9844, 76.4889, '2002-10-02T10:00:00-05:00', DEFAULT, DEFAULT, 0);
SELECT * from inprogress;

#Update existing pickup by phone number
UPDATE inprogress SET LatestLatitude = 38.9855, LatestLongitude = 76.4900, LatestTime = '1111-11-11T11:11:11-05:00'
  WHERE PhoneNumber = '5103868680';
SELECT * from inprogress;

(PhoneNumber, DeviceId, InitialLatitude, InitialLongitude, InitialTime, LatestLatitude, LatestLongitude, LatestTime, ConfirmTime, CompleteTime)
  VALUES ('5103868680', '68753A44-4D6F-1226-9C60-0050E4C00067', 38.9844, 76.4889, '2002-10-02T10:00:00-05:00', 38.9844, 76.4889, '2002-10-02T10:00:00-05:00', DEFAULT, DEFAULT);


SELECT * from inprogress;